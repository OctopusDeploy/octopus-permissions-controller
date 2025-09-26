package rules

import (
	"context"
	"crypto/sha256"
	"fmt"
	"iter"
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

func (r *Resources) getWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error) {
	wsaList := &v1beta1.WorkloadServiceAccountList{}
	err := r.client.List(ctx, wsaList)
	if err != nil {
		return nil, fmt.Errorf("failed to list workload service accounts: %w", err)
	}

	return func(yield func(*v1beta1.WorkloadServiceAccount) bool) {
		for _, v := range wsaList.Items {
			if !yield(&v) {
				return
			}
		}
	}, nil
}

func (r *Resources) ensureRoles(wsaList []*v1beta1.WorkloadServiceAccount) (map[string]rbacv1.Role, error) {
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

func (r *Resources) createRoleIfNeeded(ctx context.Context, wsa *v1beta1.WorkloadServiceAccount) (rbacv1.Role, error) {
	logger := log.FromContext(ctx).WithName("createRoleIfNeeded")

	if len(wsa.Spec.Permissions.Permissions) == 0 {
		return rbacv1.Role{}, nil
	}
	permissions := wsa.Spec.Permissions.Permissions
	namespace := wsa.GetNamespace()

	// Generate a role name based on permissions hash to ensure uniqueness
	permissionsHash := shortHash(fmt.Sprintf("%+v", permissions))
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

	// TODO: When passing ctx, client always failed with ctx cancelled errors
	err = r.client.Create(context.TODO(), &role)
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
func (r *Resources) ensureServiceAccounts(serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string) error {
	var err error
	for _, serviceAccount := range serviceAccounts {
		for _, namespace := range targetNamespaces {
			ctxWithTimeout := r.getContextWithTimeout(time.Second * 30)
			if createErr := r.createServiceAccount(ctxWithTimeout, namespace, serviceAccount); createErr != nil {
				err = multierr.Append(err, createErr)
				continue
			}
		}
	}

	return err
}

// createServiceAccount deep copies a template service account, sets the namespace, and creates the service account
func (r *Resources) createServiceAccount(
	ctx context.Context, namespace string, templateServiceAccount *corev1.ServiceAccount,
) error {
	logger := log.FromContext(ctx).WithName("createServiceAccount")

	serviceAccount := templateServiceAccount.DeepCopy()
	serviceAccount.Namespace = namespace

	err := r.client.Create(ctx, serviceAccount)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("ServiceAccount already exists", "name", serviceAccount.Name, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to create ServiceAccount %s in namespace %s: %w", serviceAccount.Name, namespace, err)
	}

	logger.Info("Created ServiceAccount", "name", serviceAccount.Name, "namespace", namespace)
	return nil
}

// ensureRoleBindings creates role bindings to connect service accounts with roles for all WSAs
func (r *Resources) ensureRoleBindings(
	ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount, createdRoles map[string]rbacv1.Role,
	wsaToServiceAccounts map[string][]string, targetNamespaces []string,
) error {
	logger := log.FromContext(ctx).WithName("ensureRoleBindings")
	var err error

	// Loop over WSAs and create role bindings for each
	for _, wsa := range wsaList {
		serviceAccounts, exists := wsaToServiceAccounts[wsa.Name]
		if !exists || len(serviceAccounts) == 0 {
			continue
		}

		var allNamespacedServiceAccounts []serviceAccountInfo
		for _, account := range serviceAccounts {
			for _, namespace := range targetNamespaces {
				allNamespacedServiceAccounts = append(allNamespacedServiceAccounts, serviceAccountInfo{
					name:      account,
					namespace: namespace,
				})
			}
		}

		ctxWithTimeout := r.getContextWithTimeout(time.Second * 30)
		if bindErr := r.createRoleBindingsForWSA(ctxWithTimeout, wsa, allNamespacedServiceAccounts, createdRoles); bindErr != nil {
			logger.Error(bindErr, "failed to create role bindings for WSA", "wsa", wsa.Name)
			err = multierr.Append(err, fmt.Errorf("failed to ensure role bindings for WSA %s: %w", wsa.Name, bindErr))
		}
	}

	return err
}

// createRoleBindingsForWSA creates all role bindings for a WSA with all its service accounts as subjects
func (r *Resources) createRoleBindingsForWSA(
	ctx context.Context, wsa *v1beta1.WorkloadServiceAccount, serviceAccounts []serviceAccountInfo,
	createdRoles map[string]rbacv1.Role,
) error {
	logger := log.FromContext(ctx).WithName("createRoleBindingsForWSA")
	var err error

	// Create subjects from all service accounts for this WSA
	subjects := make([]rbacv1.Subject, len(serviceAccounts))
	for i, sa := range serviceAccounts {
		subjects[i] = rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      sa.name,
			Namespace: sa.namespace,
		}
	}

	// WSA namespace is where role bindings will be created
	wsaNamespace := wsa.GetNamespace()

	// Create role bindings for explicit roles
	for _, roleRef := range wsa.Spec.Permissions.Roles {
		if bindErr := r.createRoleBinding(ctx, wsa, roleRef, subjects); bindErr != nil {
			err = multierr.Append(err, bindErr)
		}
	}

	// Create role binding for inline permissions (if role was created)
	if createdRole, exists := createdRoles[wsa.Name]; exists {
		roleRef := rbacv1.RoleRef{
			Kind:     "Role",
			Name:     createdRole.Name,
			APIGroup: "rbac.authorization.k8s.io",
		}
		if bindErr := r.createRoleBinding(ctx, wsa, roleRef, subjects); bindErr != nil {
			err = multierr.Append(err, bindErr)
		}
	}

	// Create cluster role bindings for cluster roles
	for _, roleRef := range wsa.Spec.Permissions.ClusterRoles {
		if bindErr := r.createClusterRoleBinding(ctx, wsa, roleRef, subjects); bindErr != nil {
			err = multierr.Append(err, bindErr)
		}
	}

	logger.Info("Created role bindings for WSA", "wsa", wsa.Name, "namespace", wsaNamespace, "serviceAccounts", len(serviceAccounts))
	return err
}

func (r *Resources) createRoleBinding(
	ctx context.Context, wsa *v1beta1.WorkloadServiceAccount, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject,
) error {
	logger := log.FromContext(ctx).WithName("createRoleBinding")

	name := fmt.Sprintf("octopus-rb-%s", shortHash(fmt.Sprintf("%s-%s", wsa.Name, roleRef.Name)))
	namespace := wsa.GetNamespace()

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			// TODO: Labels?
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}

	err := r.client.Create(ctx, roleBinding)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("RoleBinding already exists", "name", name, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to create RoleBinding %s in namespace %s: %w", name, namespace, err)
	}

	logger.Info("Created RoleBinding", "name", name, "namespace", namespace, "roleRef", roleRef.Name, "wsa", wsa.Name)
	return nil
}

func (r *Resources) createClusterRoleBinding(
	ctx context.Context, wsa *v1beta1.WorkloadServiceAccount, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject,
) error {
	logger := log.FromContext(ctx).WithName("createClusterRoleBinding")

	// TODO: This is non-namespaced, check the name is unique across the cluster
	name := fmt.Sprintf("octopus-crb-%s", shortHash(fmt.Sprintf("%s-%s", wsa.Name, roleRef.Name)))
	namespace := wsa.GetNamespace()

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			// TODO: Labels?
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}

	err := r.client.Create(ctx, clusterRoleBinding)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("ClusterRoleBinding already exists", "name", name, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to create ClusterRoleBinding %s in namespace %s: %w", name, namespace, err)
	}

	logger.Info("Created ClusterRoleBinding", "name", name, "namespace", namespace, "roleRef", roleRef.Name, "wsa", wsa.Name)
	return nil
}

func (r *Resources) getContextWithTimeout(timeout time.Duration) context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	defer cancel()
	return ctx
}

// shortHash generates a hash of a string for use in labels and names
func shortHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash)[:32] // Use first 32 characters (128 bits)
}

// generateServiceAccountName generates a ServiceAccountName based on the given scope
func generateServiceAccountName(scope Scope) ServiceAccountName {
	hash := shortHash(scope.String())
	return ServiceAccountName(fmt.Sprintf("octopus-sa-%s", hash))
}

// generateServiceAccountLabels generates the expected labels for a ServiceAccount based on scope
func generateServiceAccountLabels(scope Scope) map[string]string {
	labels := map[string]string{
		PermissionsKey: "enabled",
	}

	// Hash values for labels to keep them under 63 characters
	if scope.Project != "" {
		labels[ProjectKey] = shortHash(scope.Project)
	}
	if scope.Environment != "" {
		labels[EnvironmentKey] = shortHash(scope.Environment)
	}
	if scope.Tenant != "" {
		labels[TenantKey] = shortHash(scope.Tenant)
	}
	if scope.Step != "" {
		labels[StepKey] = shortHash(scope.Step)
	}
	if scope.Space != "" {
		labels[SpaceKey] = shortHash(scope.Space)
	}

	return labels
}

// generateExpectedAnnotations generates the expected annotations for a ServiceAccount
func generateExpectedAnnotations(scope Scope) map[string]string {
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
