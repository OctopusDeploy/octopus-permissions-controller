package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var stateSyncLog = logf.Log.WithName("statesync")

var (
	stateRebuildsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "state_sync_rebuilds_total",
		Help: "Total number of in-memory state rebuilds performed by this replica",
	})

	stateRebuildFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "state_sync_rebuild_failures_total",
		Help: "Total number of in-memory state rebuilds that failed on this replica",
	})

	stateRebuildDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "state_sync_rebuild_duration_seconds",
		Help:    "Duration of an in-memory state rebuild",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
	})
)

// StateSyncer keeps a non-leader replica's scope-to-SA mappings current. Webhooks are
// served by every replica but reconciliation only runs on the leader, so without this
// non-leaders answer pod admission from an empty map. The mappings are deterministic
// given the WSA/CWSA set, so replicas don't need to coordinate.
//
// Stands down once elected: the execution stage publishes mappings only after creating
// the service accounts they name, and refreshing would undercut that ordering.
type StateSyncer struct {
	engine    *rules.InMemoryEngine
	debouncer Debouncer

	// closed on becoming leader, and also at startup when leader election is disabled
	elected <-chan struct{}

	// both are written to without blocking, so a rebuild in flight can't stall watch handlers
	triggerCh chan struct{}
	rebuildCh chan struct{}

	synced atomic.Bool
}

func NewStateSyncer(
	engine *rules.InMemoryEngine, debounceInterval time.Duration, elected <-chan struct{},
) *StateSyncer {
	rebuildCh := make(chan struct{}, 1)
	s := &StateSyncer{
		engine:    engine,
		elected:   elected,
		triggerCh: make(chan struct{}, 1),
		rebuildCh: rebuildCh,
	}
	s.debouncer = NewDebouncer(debounceInterval, func() {
		select {
		case rebuildCh <- struct{}{}:
		default:
		}
	})
	return s
}

// Must stay implemented and false. A Runnable implementing neither this nor
// warmupRunnable lands in the manager's leader-election group by default.
func (s *StateSyncer) NeedLeaderElection() bool {
	return false
}

func (s *StateSyncer) Start(ctx context.Context) error {
	// non-leader-elected runnables start after cache sync, so this reads a warm cache
	if err := s.rebuild(ctx); err != nil {
		return fmt.Errorf("failed to build initial state: %w", err)
	}

	s.debouncer.Start(ctx)
	stateSyncLog.Info("StateSyncer started")

	for {
		select {
		case <-ctx.Done():
			stateSyncLog.Info("StateSyncer stopped")
			return nil
		case <-s.elected:
			stateSyncLog.Info("Elected leader, handing state ownership to reconciliation")
			return nil
		case <-s.triggerCh:
			s.debouncer.Debounce()
		case <-s.rebuildCh:
			if err := s.rebuild(ctx); err != nil {
				stateSyncLog.Error(err, "Failed to rebuild state")
			}
		}
	}
}

// Trigger requests a debounced rebuild. Never blocks: a rebuild recomputes everything,
// so an already-pending request covers whatever arrives behind it.
func (s *StateSyncer) Trigger() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

func (s *StateSyncer) rebuild(ctx context.Context) error {
	start := time.Now()
	if err := s.engine.RefreshScopeMappings(ctx); err != nil {
		stateRebuildFailuresTotal.Inc()
		return err
	}

	stateRebuildDuration.Observe(time.Since(start).Seconds())
	stateRebuildsTotal.Inc()
	s.synced.Store(true)
	return nil
}

func (s *StateSyncer) HasSynced() bool {
	return s.synced.Load()
}

// ReadyzCheck keeps a replica out of the webhook Service endpoints until it can
// actually resolve a scope.
func (s *StateSyncer) ReadyzCheck(_ *http.Request) error {
	if !s.HasSynced() {
		return errors.New("in-memory state has not been built yet")
	}
	return nil
}
