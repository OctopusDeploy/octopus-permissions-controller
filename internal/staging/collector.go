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
	eventMap         map[types.NamespacedName]*EventInfo
	debounceInterval time.Duration
	maxBatchSize     int

	mu             sync.RWMutex
	batchReadyCh   chan []*EventInfo
	batchTriggerCh chan struct{}
	eventDebouncer *Debouncer
}

func NewEventCollector(debounceInterval time.Duration, maxBatchSize int) *EventCollector {
	return &EventCollector{
		eventMap:         make(map[types.NamespacedName]*EventInfo),
		debounceInterval: debounceInterval,
		maxBatchSize:     maxBatchSize,
		batchReadyCh:     make(chan []*EventInfo, 10),
		batchTriggerCh:   make(chan struct{}, 1),
	}
}

func (ec *EventCollector) Start(ctx context.Context) error {
	ec.eventDebouncer = NewDebouncer(ctx, ec.debounceInterval, func() {
		ec.batchTriggerCh <- struct{}{}
	})

	log.Info("EventCollector started", "debounceInterval", ec.debounceInterval, "maxBatchSize", ec.maxBatchSize)

	for {
		select {
		case <-ctx.Done():
			close(ec.batchReadyCh)
			log.Info("EventCollector stopped")
			return nil
		case <-ec.batchTriggerCh:
			ec.FlushEvents()
		}
	}
}

func (ec *EventCollector) AddEvent(event *EventInfo) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.eventMap[event.Resource.GetNamespacedName()] = event
	eventsCollected.Inc()
	ec.eventDebouncer.Debounce()

	if len(ec.eventMap) >= ec.maxBatchSize {
		ec.batchTriggerCh <- struct{}{}
	}
}

// FlushEvents forces any pending events to be sent as a batch immediately.
func (ec *EventCollector) FlushEvents() {
	batch := ec.collectEventsIntoBatch()
	if batch != nil {
		ec.sendBatch(batch)
	}
}

// collectEventsIntoBatch extracts events from eventMap and clears the map.
func (ec *EventCollector) collectEventsIntoBatch() []*EventInfo {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if len(ec.eventMap) == 0 {
		return nil
	}

	batch := make([]*EventInfo, 0, len(ec.eventMap))
	for _, event := range ec.eventMap {
		batch = append(batch, event)
	}

	ec.eventMap = make(map[types.NamespacedName]*EventInfo)
	return batch
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
			key := event.Resource.GetNamespacedName()
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
