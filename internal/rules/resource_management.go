package rules

import (
	"context"
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

// serviceAccountInfo tracks a service account name and namespace
type serviceAccountInfo struct {
	name      string
	namespace string
}

// ResourceManagement defines the interface for creating and managing Kubernetes resources
type ResourceManagement interface {
	GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error)
	EnsureRoles(ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount) (map[string]rbacv1.Role, error)
	EnsureServiceAccounts(ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string) error
	EnsureRoleBindings(ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount, createdRoles map[string]rbacv1.Role, wsaToServiceAccounts map[string][]string, targetNamespaces []string) error
}

type ResourceManagementService struct {
	client client.Client
}

func NewResourceManagementService(newClient client.Client) ResourceManagementService {
	return ResourceManagementService{
		client: newClient,
	}
}

func (r ResourceManagementService) GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error) {
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

func (r ResourceManagementService) EnsureRoles(
	ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount,
) (map[string]rbacv1.Role, error) {
	createdRoles := make(map[string]rbacv1.Role)
	var err error

	for _, wsa := range wsaList {
		ctxWithTimeout, cancel := r.getContextWithTimeout(time.Second * 30)
		if role, createErr := r.createRoleIfNeeded(ctxWithTimeout, wsa); createErr != nil {
			err = multierr.Append(err, createErr)
		} else if role.Name != "" {
			createdRoles[wsa.Name] = role
		}
		cancel()
	}
	return createdRoles, err
}

// EnsureServiceAccounts creates service accounts for all scopes in all target namespaces
func (r ResourceManagementService) EnsureServiceAccounts(
	ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string,
) error {
	var err error
	for _, serviceAccount := range serviceAccounts {
		for _, namespace := range targetNamespaces {
			ctxWithTimeout, cancel := r.getContextWithTimeout(time.Second * 30)
			if createErr := r.createServiceAccount(ctxWithTimeout, namespace, serviceAccount); createErr != nil {
				err = multierr.Append(err, createErr)
				cancel()
				continue
			}
			cancel()
		}
	}

	return err
}

// EnsureRoleBindings creates role bindings to connect service accounts with roles for all WSAs
func (r ResourceManagementService) EnsureRoleBindings(
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

		ctxWithTimeout, cancel := r.getContextWithTimeout(time.Second * 30)
		if bindErr := r.createRoleBindingsForWSA(ctxWithTimeout, wsa, allNamespacedServiceAccounts, createdRoles); bindErr != nil {
			logger.Error(bindErr, "failed to create role bindings for WSA", "wsa", wsa.Name)
			err = multierr.Append(err, fmt.Errorf("failed to ensure role bindings for WSA %s: %w", wsa.Name, bindErr))
		}
		cancel()
	}

	return err
}

// Helper methods for ResourceManagementService

func (r ResourceManagementService) getContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return ctx, cancel
}

func (r ResourceManagementService) createRoleIfNeeded(ctx context.Context, wsa *v1beta1.WorkloadServiceAccount) (rbacv1.Role, error) {
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

// createServiceAccount deep copies a template service account, sets the namespace, and creates the service account
func (r ResourceManagementService) createServiceAccount(
	ctx context.Context, namespace string, templateServiceAccount *corev1.ServiceAccount,
) error {
	logger := log.FromContext(ctx).WithName("createServiceAccount")

	serviceAccount := templateServiceAccount.DeepCopy()
	serviceAccount.ObjectMeta.Namespace = namespace //nolint:staticcheck // The namespace must be set in the ObjectMeta

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

// createRoleBindingsForWSA creates all role bindings for a WSA with all its service accounts as subjects
func (r ResourceManagementService) createRoleBindingsForWSA(
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

func (r ResourceManagementService) createRoleBinding(
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

func (r ResourceManagementService) createClusterRoleBinding(
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
