package stages

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-set/v3"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/staging"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

type ExecutionStage struct {
	engine   *rules.InMemoryEngine
	recorder record.EventRecorder
}

func NewExecutionStage(engine *rules.InMemoryEngine, recorder record.EventRecorder) *ExecutionStage {
	return &ExecutionStage{
		engine:   engine,
		recorder: recorder,
	}
}

func (es *ExecutionStage) Name() string {
	return "execution"
}

func (es *ExecutionStage) Execute(ctx context.Context, batch *staging.Batch) error {
	log.Info("Executing reconciliation plan", "batchID", batch.ID)

	if batch.Plan == nil {
		return fmt.Errorf("batch plan is nil")
	}

	targetNamespaces := batch.Plan.TargetNamespaces
	if len(targetNamespaces) == 0 {
		targetNamespaces = es.engine.GetTargetNamespaces()
	}

	if len(targetNamespaces) == 0 {
		log.Info("No target namespaces configured, skipping resource creation")
		return nil
	}

	allResources := batch.Plan.AllResources

	log.V(1).Info("Executing reconciliation",
		"batchID", batch.ID,
		"resourceCount", len(allResources),
		"namespaceCount", len(targetNamespaces))

	var createdRoles map[string]rbacv1.Role
	if len(allResources) > 0 {
		log.V(1).Info("Ensuring roles", "resourceCount", len(allResources))
		var err error
		createdRoles, err = es.engine.EnsureRoles(ctx, allResources)
		if err != nil {
			return fmt.Errorf("failed to ensure roles: %w", err)
		}

		log.V(1).Info("Ensuring service accounts",
			"accountCount", len(batch.Plan.UniqueAccounts),
			"namespaceCount", len(targetNamespaces))
		if err := es.engine.EnsureServiceAccounts(ctx, batch.Plan.UniqueAccounts, targetNamespaces); err != nil {
			return fmt.Errorf("failed to ensure service accounts: %w", err)
		}

		if err := es.ensureRoleBindings(ctx, batch, allResources, createdRoles, targetNamespaces); err != nil {
			return fmt.Errorf("failed to ensure role bindings: %w", err)
		}
	}

	if err := es.garbageCollect(ctx, batch, allResources, targetNamespaces); err != nil {
		return fmt.Errorf("failed to garbage collect: %w", err)
	}

	if err := es.engine.ApplyBatchPlan(ctx, batch.Plan); err != nil {
		return fmt.Errorf("failed to update in-memory state: %w", err)
	}

	log.V(1).Info("In-memory state updated", "batchID", batch.ID)

	es.recordReconcileEvents(batch, len(batch.Plan.UniqueAccounts), len(allResources), len(targetNamespaces))

	log.Info("Execution completed successfully", "batchID", batch.ID)
	return nil
}

func (es *ExecutionStage) recordReconcileEvents(batch *staging.Batch, saCount, resourceCount, nsCount int) {
	if es.recorder == nil {
		return
	}

	message := fmt.Sprintf("Reconciled %d ServiceAccounts across %d namespaces with %d resources",
		saCount, nsCount, resourceCount)

	for _, resource := range batch.Resources {
		if obj, ok := resource.GetOwnerObject().(runtime.Object); ok {
			es.recorder.Event(obj, corev1.EventTypeNormal, "Reconciled", message)
		}
	}
}

func (es *ExecutionStage) ensureRoleBindings(ctx context.Context, batch *staging.Batch, allResources []rules.WSAResource, createdRoles map[string]rbacv1.Role, targetNamespaces []string) error {
	wsaToServiceAccountNames := make(map[string][]string, len(batch.Plan.WSAToSANames))
	for wsa, saNames := range batch.Plan.WSAToSANames {
		names := make([]string, len(saNames))
		for i, name := range saNames {
			names[i] = string(name)
		}
		wsaToServiceAccountNames[wsa] = names
	}

	log.V(1).Info("Ensuring role bindings",
		"resourceCount", len(allResources),
		"namespaceCount", len(targetNamespaces))
	return es.engine.EnsureRoleBindings(ctx, allResources, createdRoles, wsaToServiceAccountNames, targetNamespaces)
}

func (es *ExecutionStage) garbageCollect(ctx context.Context, batch *staging.Batch, allResources []rules.WSAResource, targetNamespaces []string) error {
	log.V(1).Info("Running garbage collection",
		"expectedSACount", len(batch.Plan.UniqueAccounts),
		"resourceCount", len(allResources),
		"namespaceCount", len(targetNamespaces))

	expectedSAs := set.New[string](len(batch.Plan.UniqueAccounts))
	for _, sa := range batch.Plan.UniqueAccounts {
		expectedSAs.Insert(sa.Name)
	}

	targetNSSet := set.New[string](len(targetNamespaces))
	for _, ns := range targetNamespaces {
		targetNSSet.Insert(ns)
	}

	if _, err := es.engine.GarbageCollectServiceAccounts(ctx, expectedSAs, targetNSSet); err != nil {
		log.Error(err, "Failed to garbage collect service accounts")
		return fmt.Errorf("failed to garbage collect service accounts: %w", err)
	}

	if err := es.engine.GarbageCollectRoles(ctx, allResources); err != nil {
		log.Error(err, "Failed to garbage collect roles")
		return fmt.Errorf("failed to garbage collect roles: %w", err)
	}

	if err := es.engine.GarbageCollectRoleBindings(ctx, allResources, targetNamespaces); err != nil {
		log.Error(err, "Failed to garbage collect role bindings")
		return fmt.Errorf("failed to garbage collect role bindings: %w", err)
	}

	if err := es.engine.GarbageCollectClusterRoleBindings(ctx, allResources); err != nil {
		log.Error(err, "Failed to garbage collect cluster role bindings")
		return fmt.Errorf("failed to garbage collect cluster role bindings: %w", err)
	}

	log.V(1).Info("Garbage collection completed")
	return nil
}
