package stages

import (
	"context"
	"fmt"
	"sync"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/staging"
	"golang.org/x/sync/semaphore"
)

type ExecutionStage struct {
	engine         *rules.InMemoryEngine
	maxConcurrency int64
}

func NewExecutionStage(engine *rules.InMemoryEngine, maxConcurrency int) *ExecutionStage {
	return &ExecutionStage{
		engine:         engine,
		maxConcurrency: int64(maxConcurrency),
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

	if err := es.engine.ApplyBatchPlan(ctx, batch.Plan); err != nil {
		return fmt.Errorf("failed to update in-memory state: %w", err)
	}

	log.V(1).Info("In-memory state updated", "batchID", batch.ID)

	targetNamespaces := es.engine.GetTargetNamespaces()
	if len(targetNamespaces) == 0 {
		log.Info("No target namespaces configured, skipping resource creation")
		return nil
	}

	sem := semaphore.NewWeighted(es.maxConcurrency)
	errCh := make(chan error, 3)
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		if err := sem.Acquire(ctx, 1); err != nil {
			errCh <- fmt.Errorf("failed to acquire semaphore for roles: %w", err)
			return
		}
		defer sem.Release(1)

		if err := es.ensureRoles(ctx, batch); err != nil {
			errCh <- fmt.Errorf("failed to ensure roles: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := sem.Acquire(ctx, 1); err != nil {
			errCh <- fmt.Errorf("failed to acquire semaphore for service accounts: %w", err)
			return
		}
		defer sem.Release(1)

		if err := es.ensureServiceAccounts(ctx, batch, targetNamespaces); err != nil {
			errCh <- fmt.Errorf("failed to ensure service accounts: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := sem.Acquire(ctx, 1); err != nil {
			errCh <- fmt.Errorf("failed to acquire semaphore for role bindings: %w", err)
			return
		}
		defer sem.Release(1)

		if err := es.ensureRoleBindings(ctx, batch, targetNamespaces); err != nil {
			errCh <- fmt.Errorf("failed to ensure role bindings: %w", err)
		}
	}()

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var errors []error
	for err := range errCh {
		if err != nil {
			log.Error(err, "Error during parallel execution")
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("execution completed with %d errors: %v", len(errors), errors)
	}

	log.Info("Execution completed successfully", "batchID", batch.ID)
	return nil
}

func (es *ExecutionStage) ensureRoles(ctx context.Context, batch *staging.Batch) error {
	log.V(1).Info("Ensuring roles", "resourceCount", len(batch.Resources))
	_, err := es.engine.EnsureRoles(ctx, batch.Resources)
	return err
}

func (es *ExecutionStage) ensureServiceAccounts(ctx context.Context, batch *staging.Batch, targetNamespaces []string) error {
	log.V(1).Info("Ensuring service accounts", "accountCount", len(batch.Plan.UniqueAccounts), "namespaceCount", len(targetNamespaces))
	return es.engine.EnsureServiceAccounts(ctx, batch.Plan.UniqueAccounts, targetNamespaces)
}

func (es *ExecutionStage) ensureRoleBindings(ctx context.Context, batch *staging.Batch, targetNamespaces []string) error {
	wsaToServiceAccountNames := make(map[string][]string, len(batch.Plan.WSAToSANames))
	for wsa, saNames := range batch.Plan.WSAToSANames {
		names := make([]string, len(saNames))
		for i, name := range saNames {
			names[i] = string(name)
		}
		wsaToServiceAccountNames[wsa] = names
	}

	log.V(1).Info("Ensuring role bindings", "resourceCount", len(batch.Resources), "namespaceCount", len(targetNamespaces))
	return es.engine.EnsureRoleBindings(ctx, batch.Resources, nil, wsaToServiceAccountNames, targetNamespaces)
}
