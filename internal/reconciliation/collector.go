package reconciliation

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

	eventQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "staging_event_queue_depth",
		Help: "Current number of events pending in the collector",
	})

	batchChannelBackpressure = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_batch_channel_backpressure_total",
		Help: "Times batch channel was full (backpressure)",
	})
)

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
	batchTriggerCh := make(chan struct{}, 1)
	return &EventCollector{
		eventMap:         make(map[types.NamespacedName]*EventInfo),
		debounceInterval: debounceInterval,
		maxBatchSize:     maxBatchSize,
		batchReadyCh:     make(chan []*EventInfo, 10),
		batchTriggerCh:   batchTriggerCh,
		eventDebouncer: NewDebouncer(debounceInterval, func() {
			batchTriggerCh <- struct{}{}
		}),
	}
}

func (ec *EventCollector) Start(ctx context.Context) error {
	ec.eventDebouncer.Start(ctx)

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
	eventQueueDepth.Set(float64(len(ec.eventMap)))
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

func (ec *EventCollector) BatchChannel() <-chan []*EventInfo {
	return ec.batchReadyCh
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
	eventQueueDepth.Set(0)
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
		batchChannelBackpressure.Inc()
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
