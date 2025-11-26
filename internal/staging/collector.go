package staging

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("staging")

var (
	eventsCollected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_events_collected_total",
		Help: "Total number of reconciliation events collected",
	})

	eventsDeduplicated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_events_deduplicated_total",
		Help: "Total number of duplicate events filtered out",
	})

	batchSizeHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "staging_batch_size",
		Help:    "Size of reconciliation batches processed",
		Buckets: prometheus.LinearBuckets(1, 10, 20), // 1-200 with step 10
	})
)

type EventType string

const (
	EventTypeCreate EventType = "Create"
	EventTypeUpdate EventType = "Update"
	EventTypeDelete EventType = "Delete"
)

type EventInfo struct {
	Resource        rules.WSAResource
	EventType       EventType
	Generation      int64
	ResourceVersion string
	Timestamp       time.Time
}

type EventCollector struct {
	eventMap              map[types.NamespacedName]*EventInfo
	generationCache       map[types.NamespacedName]int64
	generationCacheAccess map[types.NamespacedName]time.Time
	debounceInterval      time.Duration
	maxBatchSize          int

	mu           sync.RWMutex
	batchReadyCh chan []*EventInfo
}

func NewEventCollector(debounceInterval time.Duration, maxBatchSize int) *EventCollector {
	return &EventCollector{
		eventMap:              make(map[types.NamespacedName]*EventInfo),
		generationCache:       make(map[types.NamespacedName]int64),
		generationCacheAccess: make(map[types.NamespacedName]time.Time),
		debounceInterval:      debounceInterval,
		maxBatchSize:          maxBatchSize,
		batchReadyCh:          make(chan []*EventInfo, 10),
	}
}

func (ec *EventCollector) AddEvent(event *EventInfo) bool {
	ec.mu.Lock()

	key := types.NamespacedName{
		Namespace: event.Resource.GetNamespace(),
		Name:      event.Resource.GetName(),
	}

	//if lastGen, exists := ec.generationCache[key]; exists {
	//	if event.Generation <= lastGen {
	//		eventsDeduplicated.Inc()
	//		ec.mu.Unlock()
	//		return false
	//	}
	//}

	ec.eventMap[key] = event
	ec.generationCache[key] = event.Generation
	ec.generationCacheAccess[key] = time.Now()
	eventsCollected.Inc()

	shouldTrigger := len(ec.eventMap) >= ec.maxBatchSize
	if shouldTrigger {
		batch := ec.prepareBatch()
		ec.mu.Unlock()
		ec.sendBatch(batch)
	} else {
		ec.mu.Unlock()
	}

	return true
}

func (ec *EventCollector) Start(ctx context.Context) error {
	ticker := time.NewTicker(ec.debounceInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	log.Info("EventCollector started", "debounceInterval", ec.debounceInterval, "maxBatchSize", ec.maxBatchSize)

	for {
		select {
		case <-ctx.Done():
			close(ec.batchReadyCh)
			log.Info("EventCollector stopped")
			return nil
		case <-ticker.C:
			ec.mu.Lock()
			var batch []*EventInfo
			if len(ec.eventMap) > 0 {
				batch = ec.prepareBatch()
			}
			ec.mu.Unlock()

			if batch != nil {
				ec.sendBatch(batch)
			}
		case <-cleanupTicker.C:
			ec.cleanupStaleEntries()
		}
	}
}

func (ec *EventCollector) cleanupStaleEntries() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	staleThreshold := time.Now().Add(-1 * time.Hour)
	removedCount := 0

	for key, lastAccess := range ec.generationCacheAccess {
		if lastAccess.Before(staleThreshold) {
			delete(ec.generationCache, key)
			delete(ec.generationCacheAccess, key)
			removedCount++
		}
	}

	if removedCount > 0 {
		log.V(1).Info("Cleaned up stale generation cache entries",
			"removed", removedCount,
			"remaining", len(ec.generationCache))
	}
}

// prepareBatch extracts events from eventMap and clears the map.
// Caller must hold ec.mu.Lock()
func (ec *EventCollector) prepareBatch() []*EventInfo {
	batch := make([]*EventInfo, 0, len(ec.eventMap))
	for _, event := range ec.eventMap {
		batch = append(batch, event)
	}

	ec.eventMap = make(map[types.NamespacedName]*EventInfo)
	return batch
}

// Flush forces any pending events to be sent as a batch immediately.
// Returns true if a batch was sent, false if there were no pending events.
func (ec *EventCollector) Flush() bool {
	ec.mu.Lock()
	if len(ec.eventMap) == 0 {
		ec.mu.Unlock()
		return false
	}
	batch := ec.prepareBatch()
	ec.mu.Unlock()
	ec.sendBatch(batch)
	return true
}

// sendBatch attempts to send the batch to the channel without blocking.
// This method does NOT require holding the lock.
func (ec *EventCollector) sendBatch(batch []*EventInfo) {
	batchSizeHistogram.Observe(float64(len(batch)))

	select {
	case ec.batchReadyCh <- batch:
		log.V(1).Info("Batch sent to orchestrator", "size", len(batch))
	default:
		log.Info("Batch channel full, events will be retried on next tick", "droppedBatchSize", len(batch))
		ec.mu.Lock()
		for _, event := range batch {
			key := types.NamespacedName{
				Namespace: event.Resource.GetNamespace(),
				Name:      event.Resource.GetName(),
			}
			if existing, exists := ec.eventMap[key]; !exists || event.Generation > existing.Generation {
				ec.eventMap[key] = event
			}
		}
		ec.mu.Unlock()
	}
}

func (ec *EventCollector) BatchChannel() <-chan []*EventInfo {
	return ec.batchReadyCh
}

type Batch struct {
	ID               string
	Resources        []rules.WSAResource
	StartTime        time.Time
	Plan             *ReconciliationPlan
	ValidationResult *ValidationResult
	RetryCount       int
	LastError        error
	RequeueAfter     time.Duration
}

func NewBatch(events []*EventInfo) *Batch {
	resources := make([]rules.WSAResource, len(events))
	for i, event := range events {
		resources[i] = event.Resource
	}

	return &Batch{
		ID:        uuid.New().String(),
		Resources: resources,
		StartTime: time.Now(),
	}
}

type ReconciliationPlan struct {
	ScopeToSA        map[rules.Scope]rules.ServiceAccountName
	SAToWSAMap       map[rules.ServiceAccountName]map[string]rules.WSAResource
	WSAToSANames     map[string][]rules.ServiceAccountName
	Vocabulary       *rules.GlobalVocabulary
	UniqueAccounts   []*v1.ServiceAccount
	AllResources     []rules.WSAResource
	TargetNamespaces []string
}

func (rp *ReconciliationPlan) GetScopeToSA() map[rules.Scope]rules.ServiceAccountName {
	return rp.ScopeToSA
}

func (rp *ReconciliationPlan) GetSAToWSAMap() map[rules.ServiceAccountName]map[string]rules.WSAResource {
	return rp.SAToWSAMap
}

func (rp *ReconciliationPlan) GetVocabulary() *rules.GlobalVocabulary {
	return rp.Vocabulary
}

type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

type ValidationError struct {
	Type     string
	Resource string
	Message  string
}

type ValidationWarning struct {
	Type     string
	Resource string
	Message  string
}
