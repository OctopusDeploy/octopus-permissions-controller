package stages

import (
	"context"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/staging"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("staging.planning")

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

func (ps *PlanningStage) Execute(ctx context.Context, batch *staging.Batch) error {
	log.Info("Computing reconciliation plan", "batchID", batch.ID, "resourceCount", len(batch.Resources))

	scopeComputation := rules.NewScopeComputationService(nil, nil)
	scopeMap, vocabulary := scopeComputation.ComputeScopesForWSAs(batch.Resources)
	log.V(1).Info("Computed scope map", "scopeCount", len(scopeMap))

	scopeToSA, saToWSAMap, wsaToSANames, uniqueAccounts := scopeComputation.GenerateServiceAccountMappings(scopeMap)
	log.V(1).Info("Generated service account mappings",
		"scopeCount", len(scopeToSA),
		"saCount", len(saToWSAMap),
		"uniqueAccounts", len(uniqueAccounts))

	batch.Plan = &staging.ReconciliationPlan{
		ScopeToSA:      scopeToSA,
		SAToWSAMap:     saToWSAMap,
		WSAToSANames:   convertWSAToSANames(wsaToSANames),
		Vocabulary:     &vocabulary,
		UniqueAccounts: uniqueAccounts,
	}

	log.Info("Plan computed successfully", "batchID", batch.ID, "serviceAccounts", len(batch.Plan.UniqueAccounts))
	return nil
}

func convertWSAToSANames(wsaToSANames map[string][]string) map[string][]rules.ServiceAccountName {
	result := make(map[string][]rules.ServiceAccountName, len(wsaToSANames))
	for wsa, saNames := range wsaToSANames {
		converted := make([]rules.ServiceAccountName, len(saNames))
		for i, name := range saNames {
			converted[i] = rules.ServiceAccountName(name)
		}
		result[wsa] = converted
	}
	return result
}
