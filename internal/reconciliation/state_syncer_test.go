package reconciliation

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	internaltypes "github.com/octopusdeploy/octopus-permissions-controller/internal/types"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testDebounce = 100 * time.Millisecond

// stands in for mgr.Elected() on a replica that stays a follower
func neverElected() <-chan struct{} { return make(chan struct{}) }

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return scheme
}

func testWSA(name, project string) *v1beta1.WorkloadServiceAccount {
	return &v1beta1.WorkloadServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: v1beta1.WorkloadServiceAccountSpec{
			Scope: v1beta1.WorkloadServiceAccountScope{Projects: []string{project}},
		},
	}
}

func newTestEngine(t *testing.T, objs ...client.Object) *rules.InMemoryEngine {
	t.Helper()
	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	engine := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
	return &engine
}

// Reporting true here, or dropping the method, would leave non-leaders serving pod
// admission from an empty map.
func TestStateSyncerIsNotLeaderElected(t *testing.T) {
	syncer := NewStateSyncer(newTestEngine(t), testDebounce, neverElected())
	assert.False(t, syncer.NeedLeaderElection())
}

func TestStateSyncerBuildsStateOnStart(t *testing.T) {
	// everything the syncer blocks on must be created inside the bubble, or synctest
	// won't treat it as durably blocked and Wait never returns
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		engine := newTestEngine(t, testWSA("wsa-1", "project-1"))
		syncer := NewStateSyncer(engine, testDebounce, neverElected())

		assert.False(t, syncer.HasSynced(), "should not report synced before Start")
		assert.Error(t, syncer.ReadyzCheck(nil), "readyz should fail before the first build")

		go func() { _ = syncer.Start(ctx) }()
		synctest.Wait()

		assert.True(t, syncer.HasSynced(), "should report synced after the initial build")
		assert.NoError(t, syncer.ReadyzCheck(nil), "readyz should pass after the initial build")

		// what the pod webhook reads on a non-leader replica
		sa, err := engine.GetServiceAccountForScope(internaltypes.Scope{Project: "project-1"})
		require.NoError(t, err)
		assert.NotEmpty(t, sa, "scope should resolve without any reconciliation having run")
	})
}

func TestStateSyncerFailsStartWhenInitialBuildFails(t *testing.T) {
	// a scheme lacking the WSA types can't list them
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	engine := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})

	syncer := NewStateSyncer(&engine, testDebounce, neverElected())

	err := syncer.Start(context.Background())
	require.Error(t, err, "Start must fail so the manager exits rather than serving empty state")
	assert.False(t, syncer.HasSynced())
	assert.Error(t, syncer.ReadyzCheck(nil))
}

func TestStateSyncerDebouncesTriggers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		syncer := NewStateSyncer(newTestEngine(t, testWSA("wsa-1", "project-1")), testDebounce, neverElected())

		go func() { _ = syncer.Start(ctx) }()
		synctest.Wait()

		before := testutil.ToFloat64(stateRebuildsTotal)

		for range 10 {
			syncer.Trigger()
			time.Sleep(testDebounce / 5)
		}

		time.Sleep(2 * testDebounce)
		synctest.Wait()

		// 10 triggers across 2 debounce windows collapse into 2 rebuilds, not 10
		rebuilds := testutil.ToFloat64(stateRebuildsTotal) - before
		assert.Equal(t, float64(2), rebuilds)
	})
}

func TestStateSyncerPicksUpChangesAfterTrigger(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		scheme := newTestScheme(t)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		engine := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
		syncer := NewStateSyncer(&engine, testDebounce, neverElected())

		go func() { _ = syncer.Start(ctx) }()
		synctest.Wait()

		scope := internaltypes.Scope{Project: "project-late"}
		sa, err := engine.GetServiceAccountForScope(scope)
		require.NoError(t, err)
		require.Empty(t, sa, "scope should not resolve before the WSA exists")

		require.NoError(t, fakeClient.Create(ctx, testWSA("wsa-late", "project-late")))
		syncer.Trigger()

		time.Sleep(2 * testDebounce)
		synctest.Wait()

		sa, err = engine.GetServiceAccountForScope(scope)
		require.NoError(t, err)
		assert.NotEmpty(t, sa, "scope should resolve after the debounced rebuild")
	})
}

// Trigger is called from watch handlers, which must never block on a rebuild.
func TestStateSyncerTriggerNeverBlocks(t *testing.T) {
	syncer := NewStateSyncer(newTestEngine(t), testDebounce, neverElected())

	done := make(chan struct{})
	go func() {
		// not started, so nothing is draining the channels
		for range 1000 {
			syncer.Trigger()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Trigger blocked")
	}
}

// Once elected, reconciliation owns the state: it publishes mappings only after
// creating the service accounts they name, and refreshing would undercut that.
func TestStateSyncerStopsOnceElected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		elected := make(chan struct{})
		syncer := NewStateSyncer(newTestEngine(t, testWSA("wsa-1", "project-1")), testDebounce, elected)

		stopped := make(chan struct{})
		go func() {
			_ = syncer.Start(ctx)
			close(stopped)
		}()
		synctest.Wait()

		require.True(t, syncer.HasSynced(), "should build state before election")
		before := testutil.ToFloat64(stateRebuildsTotal)

		close(elected)
		synctest.Wait()

		select {
		case <-stopped:
		default:
			t.Fatal("Start should return once elected")
		}

		syncer.Trigger()
		time.Sleep(3 * testDebounce)
		synctest.Wait()

		assert.Equal(t, before, testutil.ToFloat64(stateRebuildsTotal),
			"no refreshes should happen after election")
		assert.True(t, syncer.HasSynced(), "readiness must survive standing down")
	})
}

// saToWsaMap is reconciliation's signal that staging processed a deletion and the
// finalizer can go (see controller.handleDeletion). A refresh touching it would
// release finalizers before the leader had run GC.
func TestRefreshScopeMappingsLeavesDeletionInterlockAlone(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)

	wsa := testWSA("wsa-1", "project-1")
	wsa.Finalizers = []string{"octopus.com/serviceaccount-cleanup"}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wsa).Build()
	engine := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})

	// seed saToWsaMap the way a completed reconciliation would
	require.NoError(t, engine.RebuildStateFromCluster(ctx))
	key := types.NamespacedName{Namespace: "test-ns", Name: "wsa-1"}
	require.True(t, engine.IsWSAInMaps(key), "precondition: WSA is tracked after a full rebuild")

	// the finalizer keeps the object around with a DeletionTimestamp set, which is
	// what a full rebuild filters out
	require.NoError(t, fakeClient.Delete(ctx, wsa))
	fresh := &v1beta1.WorkloadServiceAccount{}
	require.NoError(t, fakeClient.Get(ctx, key, fresh))
	require.False(t, fresh.DeletionTimestamp.IsZero(), "precondition: WSA is mid-deletion")

	require.NoError(t, engine.RefreshScopeMappings(ctx))
	assert.True(t, engine.IsWSAInMaps(key),
		"a scope refresh must not retire the WSA from the deletion interlock")

	// a full rebuild, which only reconciliation runs, still does
	require.NoError(t, engine.RebuildStateFromCluster(ctx))
	assert.False(t, engine.IsWSAInMaps(key))
}
