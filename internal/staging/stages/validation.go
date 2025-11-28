package stages

import (
	"context"
	"fmt"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/staging"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ValidationStage struct {
	client           client.Client
	engine           *rules.InMemoryEngine
	targetNamespaces []string
}

func NewValidationStage(c client.Client, engine *rules.InMemoryEngine, targetNamespaces []string) *ValidationStage {
	return &ValidationStage{
		client:           c,
		engine:           engine,
		targetNamespaces: targetNamespaces,
	}
}

func (vs *ValidationStage) Name() string {
	return "validation"
}

func (vs *ValidationStage) Execute(ctx context.Context, batch *staging.Batch) error {
	log.Info("Validating batch", "batchID", batch.ID)

	result := &staging.ValidationResult{
		Valid:    true,
		Errors:   []staging.ValidationError{},
		Warnings: []staging.ValidationWarning{},
	}

	if err := vs.checkOwnerReferences(ctx, batch, result); err != nil {
		return fmt.Errorf("failed to check owner references: %w", err)
	}

	if err := vs.checkNamespaceExistence(ctx, result); err != nil {
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}

	batch.ValidationResult = result

	if !result.Valid {
		return fmt.Errorf("validation failed with %d errors", len(result.Errors))
	}

	if len(result.Warnings) > 0 {
		log.Info("Validation completed with warnings", "batchID", batch.ID, "warningCount", len(result.Warnings))
	} else {
		log.Info("Validation completed successfully", "batchID", batch.ID)
	}

	return nil
}

func (vs *ValidationStage) checkOwnerReferences(ctx context.Context, batch *staging.Batch, result *staging.ValidationResult) error {
	if batch.Plan == nil {
		return fmt.Errorf("batch plan is nil")
	}

	for _, sa := range batch.Plan.UniqueAccounts {
		for _, namespace := range vs.getTargetNamespaces() {
			existing := &v1.ServiceAccount{}
			err := vs.client.Get(ctx, types.NamespacedName{
				Name:      sa.Name,
				Namespace: namespace,
			}, existing)

			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("failed to get service account %s/%s: %w", namespace, sa.Name, err)
			}

			if !vs.isOctopusManaged(existing.Labels) {
				result.Errors = append(result.Errors, staging.ValidationError{
					Type:     "OwnerConflict",
					Resource: fmt.Sprintf("ServiceAccount/%s/%s", namespace, sa.Name),
					Message:  "Service account exists but is not managed by octopus-permissions-controller",
				})
				result.Valid = false
			}
		}
	}

	for _, resource := range batch.Resources {
		roleName := fmt.Sprintf("octopus-%s", resource.GetName())
		namespace := resource.GetNamespace()

		if namespace == "" {
			continue
		}

		existing := &rbacv1.Role{}
		err := vs.client.Get(ctx, types.NamespacedName{
			Name:      roleName,
			Namespace: namespace,
		}, existing)

		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get role %s/%s: %w", namespace, roleName, err)
		}

		if !vs.isOctopusManaged(existing.Labels) {
			result.Warnings = append(result.Warnings, staging.ValidationWarning{
				Type:     "OwnerConflict",
				Resource: fmt.Sprintf("Role/%s/%s", namespace, roleName),
				Message:  "Role exists but is not managed by octopus-permissions-controller",
			})
		}
	}

	return nil
}

func (vs *ValidationStage) checkNamespaceExistence(ctx context.Context, result *staging.ValidationResult) error {
	for _, namespace := range vs.getTargetNamespaces() {
		ns := &v1.Namespace{}
		err := vs.client.Get(ctx, types.NamespacedName{Name: namespace}, ns)

		if err != nil {
			if apierrors.IsNotFound(err) {
				result.Errors = append(result.Errors, staging.ValidationError{
					Type:     "NamespaceNotFound",
					Resource: fmt.Sprintf("Namespace/%s", namespace),
					Message:  fmt.Sprintf("Target namespace '%s' does not exist", namespace),
				})
				result.Valid = false
				continue
			}
			return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
		}
	}

	return nil
}

func (vs *ValidationStage) getTargetNamespaces() []string {
	if len(vs.targetNamespaces) > 0 {
		return vs.targetNamespaces
	}
	return vs.engine.GetTargetNamespaces()
}

func (vs *ValidationStage) isOctopusManaged(labels map[string]string) bool {
	return rules.IsOctopusManaged(labels)
}
