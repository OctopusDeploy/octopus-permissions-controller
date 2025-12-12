package reconciliation

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestEventCollector(t *testing.T) {
	t.Run("collector flushes batch of events", func(t *testing.T) {
		manualDebouncer := NewFakeDebouncer(func() {})
		collector := NewEventCollectorWithDebouncer(time.Second, 10, manualDebouncer)

		collector.AddEvent(createEventInfo("test1"))
		collector.AddEvent(createEventInfo("test2"))

		select {
		case batch := <-collector.BatchChannel():
			t.Fatalf("Expected no batch yet, but got batch with %d events", len(batch))
		default:
		}

		collector.FlushEvents()

		select {
		case batch := <-collector.BatchChannel():
			expectedResources := []string{
				"default/test1",
				"default/test2",
			}

			actualResources := collectResourceNames(batch)

			if diff := cmp.Diff(expectedResources, actualResources); diff != "" {
				t.Errorf("Unexpected resources in batch:\n%s", diff)
			}

		default:
			t.Fatal("Expected batch to be available after manual trigger")
		}
	})

	t.Run("events are deduplicated", func(t *testing.T) {
		manualDebouncer := NewFakeDebouncer(func() {})
		collector := NewEventCollectorWithDebouncer(time.Second, 10, manualDebouncer)

		collector.AddEvent(createEventInfo("test1"))
		newerEvent := createEventInfo("test1")
		newerEvent.Generation = 2
		collector.AddEvent(newerEvent)

		select {
		case batch := <-collector.BatchChannel():
			t.Fatalf("Expected no batch yet, but got batch with %d events", len(batch))
		default:
		}

		collector.FlushEvents()

		select {
		case batch := <-collector.BatchChannel():
			expectedResources := []string{
				"default/test1",
			}

			actualResources := collectResourceNames(batch)

			if diff := cmp.Diff(expectedResources, actualResources); diff != "" {
				t.Errorf("Unexpected resources in batch:\n%s", diff)
			}

		default:
			t.Fatal("Expected batch to be available after manual trigger")
		}
	})

	t.Run("events are preserved when there is back pressure", func(t *testing.T) {
		manualDebouncer := NewFakeDebouncer(func() {})
		collector := NewEventCollectorWithDebouncer(time.Second, 10, manualDebouncer)

		// Create back pressure by filling the batch channel
		for i := 0; i < 10; i++ {
			batch := []*EventInfo{
				createEventInfo("filler"),
			}
			collector.batchReadyCh <- batch
		}

		collector.AddEvent(createEventInfo("test1"))
		collector.AddEvent(createEventInfo("test2"))

		collector.FlushEvents()

		// Remove back pressure by draining the batch channel
		for i := 0; i < 10; i++ {
			select {
			case <-collector.BatchChannel():
				// We don't care about the filler batches
			default:
				t.Fatal("Expected batch to be available after manual trigger")
			}
		}

		select {
		case batch := <-collector.BatchChannel():
			t.Fatalf("Expected no batch yet, but got batch with %d events", len(batch))
		default:
		}
		collector.FlushEvents()

		select {
		case batch := <-collector.BatchChannel():
			expectedResources := []string{
				"default/test1",
				"default/test2",
			}

			actualResources := collectResourceNames(batch)

			if diff := cmp.Diff(expectedResources, actualResources); diff != "" {
				t.Errorf("Unexpected resources in batch:\n%s", diff)
			}

		default:
			t.Fatal("Expected batch to be available after manual trigger")
		}
	})
}

func NewEventCollectorWithDebouncer(debounceInterval time.Duration, maxBatchSize int, debouncer Debouncer) *EventCollector {
	batchTriggerCh := make(chan struct{}, 1)
	return &EventCollector{
		eventMap:         make(map[types.NamespacedName]*EventInfo),
		debounceInterval: debounceInterval,
		maxBatchSize:     maxBatchSize,
		mu:               sync.RWMutex{},
		batchReadyCh:     make(chan []*EventInfo, 10),
		batchTriggerCh:   batchTriggerCh,
		eventDebouncer:   debouncer,
	}
}

func createEventInfo(name string) *EventInfo {
	wsa := &v1beta1.WorkloadServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	resource := rules.NewWSAResource(wsa)

	return &EventInfo{
		Resource:        resource,
		EventType:       EventTypeUpdate,
		Generation:      1,
		ResourceVersion: "1",
		Timestamp:       time.Now(),
	}
}

func collectResourceNames(batch []*EventInfo) []string {
	actualResources := []string{}
	for _, event := range batch {
		actualResources = append(actualResources, event.Resource.GetNamespacedName().String())
	}
	slices.Sort(actualResources)
	return actualResources
}

type FakeDebouncer struct {
	callback func()
}

func NewFakeDebouncer(callback func()) *FakeDebouncer {
	return &FakeDebouncer{callback: callback}
}

func (m *FakeDebouncer) Start(ctx context.Context) {}

func (m *FakeDebouncer) Debounce() {}
