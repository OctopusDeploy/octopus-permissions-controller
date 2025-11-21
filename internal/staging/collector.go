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
	"k8s.io/client-go/util/workqueue"
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
	queue            workqueue.RateLimitingInterface
	eventMap         map[types.NamespacedName]*EventInfo
	generationCache  map[types.NamespacedName]int64
	debounceInterval time.Duration
	maxBatchSize     int

	mu           sync.RWMutex
	stopCh       chan struct{}
	batchReadyCh chan []*EventInfo
}

func NewEventCollector(debounceInterval time.Duration, maxBatchSize int) *EventCollector {
	return &EventCollector{
		queue:            workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		eventMap:         make(map[types.NamespacedName]*EventInfo),
		generationCache:  make(map[types.NamespacedName]int64),
		debounceInterval: debounceInterval,
		maxBatchSize:     maxBatchSize,
		stopCh:           make(chan struct{}),
		batchReadyCh:     make(chan []*EventInfo, 10),
	}
}

func (ec *EventCollector) AddEvent(event *EventInfo) bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	key := types.NamespacedName{
		Namespace: event.Resource.GetNamespace(),
		Name:      event.Resource.GetName(),
	}

	if lastGen, exists := ec.generationCache[key]; exists {
		if event.Generation <= lastGen {
			eventsDeduplicated.Inc()
			return false
		}
	}

	ec.eventMap[key] = event
	ec.generationCache[key] = event.Generation
	eventsCollected.Inc()

	if len(ec.eventMap) >= ec.maxBatchSize {
		ec.triggerBatch()
	}

	return true
}

func (ec *EventCollector) Start(ctx context.Context) error {
	ticker := time.NewTicker(ec.debounceInterval)
	defer ticker.Stop()

	log.Info("EventCollector started", "debounceInterval", ec.debounceInterval, "maxBatchSize", ec.maxBatchSize)

	for {
		select {
		case <-ctx.Done():
			close(ec.stopCh)
			close(ec.batchReadyCh)
			ec.queue.ShutDown()
			log.Info("EventCollector stopped")
			return nil
		case <-ticker.C:
			ec.mu.Lock()
			if len(ec.eventMap) > 0 {
				ec.triggerBatch()
			}
			ec.mu.Unlock()
		}
	}
}

func (ec *EventCollector) triggerBatch() {
	batch := make([]*EventInfo, 0, len(ec.eventMap))
	for _, event := range ec.eventMap {
		batch = append(batch, event)
	}

	batchSizeHistogram.Observe(float64(len(batch)))

	select {
	case ec.batchReadyCh <- batch:
		ec.eventMap = make(map[types.NamespacedName]*EventInfo)
	default:
		log.Info("Batch channel full, events will be retried on next tick")
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
	ScopeToSA      map[rules.Scope]rules.ServiceAccountName
	SAToWSAMap     map[rules.ServiceAccountName]map[string]rules.WSAResource
	WSAToSANames   map[string][]rules.ServiceAccountName
	Vocabulary     *rules.GlobalVocabulary
	UniqueAccounts []*v1.ServiceAccount
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
