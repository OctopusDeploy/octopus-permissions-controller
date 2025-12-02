package stages

import (
	"context"
	"fmt"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/reconciliation"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("staging.stages")

type PlanningStage struct {
	engine *rules.InMemoryEngine
}

func NewPlanningStage(engine *rules.InMemoryEngine) *PlanningStage {
	return &PlanningStage{
		engine: engine,
	}
}

func (ps *PlanningStage) Name() string {
	return "planning"
}

func (ps *PlanningStage) Execute(ctx context.Context, batch *reconciliation.Batch) error {
	log.Info("Computing reconciliation plan", "batchID", batch.ID, "batchResourceCount", len(batch.Resources))

	allResources, err := ps.getAllResources(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all resources: %w", err)
	}

	log.V(1).Info("Queried all cluster resources", "totalResources", len(allResources))

	targetNamespaces, err := ps.engine.GetOrDiscoverTargetNamespaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to get target namespaces: %w", err)
	}

	scopeComputation := rules.NewScopeComputationService(nil, nil)
	scopeMap, vocabulary := scopeComputation.ComputeScopesForWSAs(allResources)
	log.V(1).Info("Computed scope map for all resources", "scopeCount", len(scopeMap))

	scopeToSA, saToWSAMap, wsaToSANames, uniqueAccounts := scopeComputation.GenerateServiceAccountMappings(scopeMap)
	log.V(1).Info("Generated service account mappings",
		"scopeCount", len(scopeToSA),
		"saCount", len(saToWSAMap),
		"uniqueAccounts", len(uniqueAccounts))

	batch.Plan = &reconciliation.Plan{
		ScopeToSA:        scopeToSA,
		SAToWSAMap:       saToWSAMap,
		WSAToSANames:     convertWSAToSANames(wsaToSANames),
		Vocabulary:       &vocabulary,
		UniqueAccounts:   uniqueAccounts,
		AllResources:     allResources,
		TargetNamespaces: targetNamespaces,
	}

	batch.Resources = updateBatchResourceVersions(batch.Resources, allResources)

	log.Info("Plan computed successfully", "batchID", batch.ID, "serviceAccounts", len(batch.Plan.UniqueAccounts))
	return nil
}

func updateBatchResourceVersions(batchResources, freshResources []rules.WSAResource) []rules.WSAResource {
	freshByKey := make(map[types.NamespacedName]rules.WSAResource, len(freshResources))
	for _, r := range freshResources {
		freshByKey[r.GetNamespacedName()] = r
	}

	updated := make([]rules.WSAResource, 0, len(batchResources))
	for _, r := range batchResources {
		if fresh, ok := freshByKey[r.GetNamespacedName()]; ok {
			updated = append(updated, fresh)
		} else {
			updated = append(updated, r)
		}
	}
	return updated
}

func (ps *PlanningStage) getAllResources(ctx context.Context) ([]rules.WSAResource, error) {
	allResources := make([]rules.WSAResource, 0)

	wsaIter, err := ps.engine.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list WorkloadServiceAccounts: %w", err)
	}

	for wsa := range wsaIter {
		if wsa.DeletionTimestamp.IsZero() {
			allResources = append(allResources, rules.NewWSAResource(wsa))
		}
	}

	cwsaIter, err := ps.engine.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list ClusterWorkloadServiceAccounts: %w", err)
	}

	for cwsa := range cwsaIter {
		if cwsa.DeletionTimestamp.IsZero() {
			allResources = append(allResources, rules.NewClusterWSAResource(cwsa))
		}
	}

	return allResources, nil
}

func convertWSAToSANames(wsaToSANames map[types.NamespacedName][]string) map[types.NamespacedName][]rules.ServiceAccountName {
	result := make(map[types.NamespacedName][]rules.ServiceAccountName, len(wsaToSANames))
	for wsa, saNames := range wsaToSANames {
		converted := make([]rules.ServiceAccountName, len(saNames))
		for i, name := range saNames {
			converted[i] = rules.ServiceAccountName(name)
		}
		result[wsa] = converted
	}
	return result
}
