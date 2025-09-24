package rules

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Constants for metadata keys (used in both labels and annotations)
const (
	MetadataNamespace = "agent.octopus.com"
	PermissionsKey    = MetadataNamespace + "/permissions"
	ProjectKey        = MetadataNamespace + "/project"
	EnvironmentKey    = MetadataNamespace + "/environment"
	TenantKey         = MetadataNamespace + "/tenant"
	StepKey           = MetadataNamespace + "/step"
	SpaceKey          = MetadataNamespace + "/space"
)

// serviceAccountInfo tracks a service account name and namespace
type serviceAccountInfo struct {
	name      string
	namespace string
}

type Resources struct {
	client client.Client
}

func NewResources(client client.Client) Resources {
	return Resources{
		client: client,
	}
}

func (r *Resources) getWorkloadServiceAccounts(ctx context.Context) ([]v1beta1.WorkloadServiceAccount, error) {
	wsaList := &v1beta1.WorkloadServiceAccountList{}
	err := r.client.List(ctx, wsaList)
	if err != nil {
		return nil, fmt.Errorf("failed to list workload service accounts: %w", err)
	}

	return wsaList.Items, nil
}

func (r *Resources) ensureRoles(wsaList []v1beta1.WorkloadServiceAccount) (map[string]rbacv1.Role, error) {
	createdRoles := make(map[string]rbacv1.Role)
	var err error

	for _, wsa := range wsaList {
		ctxWithTimeout := r.getContextWithTimeout(time.Second * 30)
		if role, createErr := r.createRoleIfNeeded(ctxWithTimeout, wsa); createErr != nil {
			err = multierr.Append(err, createErr)
		} else if role.Name != "" {
			createdRoles[wsa.Name] = role
		}
	}
	return createdRoles, err
}

func (r *Resources) createRoleIfNeeded(ctx context.Context, wsa v1beta1.WorkloadServiceAccount) (rbacv1.Role, error) {
	logger := log.FromContext(ctx).WithName("createRoleIfNeeded")

	if len(wsa.Spec.Permissions.Permissions) == 0 {
		return rbacv1.Role{}, nil
	}
	permissions := wsa.Spec.Permissions.Permissions
	namespace := wsa.GetNamespace()

	// Generate a role name based on permissions hash to ensure uniqueness
	permissionsHash := r.shortHash(fmt.Sprintf("%+v", permissions))
	roleName := fmt.Sprintf("octopus-role-%s", permissionsHash)

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels: map[string]string{
				PermissionsKey: "enabled",
			},
		},
		Rules: permissions,
	}

	existingRole := rbacv1.Role{}
	err := r.client.Get(ctx, client.ObjectKeyFromObject(&role), &existingRole)
	if err == nil {
		logger.Info("Role already exists", "name", roleName)
		return existingRole, nil
	}

	err = r.client.Create(ctx, &role)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("Role already exists", "name", roleName)
			return role, nil
		}
		return rbacv1.Role{}, fmt.Errorf("failed to create Role %s: %w", roleName, err)
	}

	logger.Info("Created Role", "name", roleName, "permissions", len(permissions))
	return role, nil
}

// ensureServiceAccounts creates service accounts for all scopes in all target namespaces
func (r *Resources) ensureServiceAccounts(scopeMap map[Scope]map[string]v1beta1.WorkloadServiceAccount, targetNamespaces []string) (map[string][]serviceAccountInfo, error) {
	wsaToServiceAccounts := make(map[string][]serviceAccountInfo)
	var err error

	for scope, wsaMap := range scopeMap {
		serviceAccountName := r.generateServiceAccountName(scope)

		// Create service account in all target namespaces
		var createdSAs []serviceAccountInfo
		for _, namespace := range targetNamespaces {
			ctxWithTimeout := r.getContextWithTimeout(time.Second * 30)
			if createErr := r.createServiceAccount(ctxWithTimeout, namespace, scope); createErr != nil {
				err = multierr.Append(err, createErr)
				continue
			}
			createdSAs = append(createdSAs, serviceAccountInfo{
				name:      string(serviceAccountName),
				namespace: namespace,
			})
		}

		// Map these service accounts to all WSAs that contribute to this scope
		for wsaName := range wsaMap {
			wsaToServiceAccounts[wsaName] = append(wsaToServiceAccounts[wsaName], createdSAs...)
		}
	}

	return wsaToServiceAccounts, err
}

// createServiceAccount creates a ServiceAccount for the given scope with proper labels and annotations
func (r *Resources) createServiceAccount(ctx context.Context, namespace string, scope Scope) error {
	logger := log.FromContext(ctx).WithName("createServiceAccount")

	serviceAccountName := r.generateServiceAccountName(scope)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        string(serviceAccountName),
			Namespace:   namespace,
			Labels:      r.generateServiceAccountLabels(scope),
			Annotations: r.generateExpectedAnnotations(scope),
		},
	}

	err := r.client.Create(ctx, serviceAccount)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("ServiceAccount already exists", "name", serviceAccountName, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to create ServiceAccount %s in namespace %s: %w", serviceAccountName, namespace, err)
	}

	logger.Info("Created ServiceAccount", "name", serviceAccountName, "namespace", namespace, "scope", scope.String())
	return nil
}

func (r *Resources) getContextWithTimeout(timeout time.Duration) context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	defer cancel()
	return ctx
}

// shortHash generates a hash of a string for use in labels and names
func (r *Resources) shortHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash)[:32] // Use first 32 characters (128 bits)
}

// generateServiceAccountName generates a ServiceAccountName based on the given scope
func (r *Resources) generateServiceAccountName(scope Scope) ServiceAccountName {
	hash := r.shortHash(scope.String())
	return ServiceAccountName(fmt.Sprintf("octopus-sa-%s", hash))
}

// generateServiceAccountLabels generates the expected labels for a ServiceAccount based on scope
func (r *Resources) generateServiceAccountLabels(scope Scope) map[string]string {
	labels := map[string]string{
		PermissionsKey: "enabled",
	}

	// Hash values for labels to keep them under 63 characters
	if scope.Project != "" {
		labels[ProjectKey] = r.shortHash(scope.Project)
	}
	if scope.Environment != "" {
		labels[EnvironmentKey] = r.shortHash(scope.Environment)
	}
	if scope.Tenant != "" {
		labels[TenantKey] = r.shortHash(scope.Tenant)
	}
	if scope.Step != "" {
		labels[StepKey] = r.shortHash(scope.Step)
	}
	if scope.Space != "" {
		labels[SpaceKey] = r.shortHash(scope.Space)
	}

	return labels
}

// generateExpectedAnnotations generates the expected annotations for a ServiceAccount
func (r *Resources) generateExpectedAnnotations(scope Scope) map[string]string {
	annotations := make(map[string]string)

	if scope.Project != "" {
		annotations[ProjectKey] = scope.Project
	}
	if scope.Environment != "" {
		annotations[EnvironmentKey] = scope.Environment
	}
	if scope.Tenant != "" {
		annotations[TenantKey] = scope.Tenant
	}
	if scope.Step != "" {
		annotations[StepKey] = scope.Step
	}
	if scope.Space != "" {
		annotations[SpaceKey] = scope.Space
	}

	return annotations
}
