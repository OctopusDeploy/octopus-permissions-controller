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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/hashicorp/go-set/v3"
)

// serviceAccountInfo tracks a service account name and namespace
type serviceAccountInfo struct {
	name      string
	namespace string
}

const serviceAccountFinalizer = "octopus.com/serviceaccount"

// ResourceManagement defines the interface for creating and managing Kubernetes resources
type ResourceManagement interface {
	GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error)
	GetClusterWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.ClusterWorkloadServiceAccount], error)
	GetServiceAccounts(ctx context.Context) (iter.Seq[*corev1.ServiceAccount], error)
	GetRoles(ctx context.Context) (iter.Seq[*rbacv1.Role], error)
	GetClusterRoles(ctx context.Context) (iter.Seq[*rbacv1.ClusterRole], error)
	GetRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.RoleBinding], error)
	GetClusterRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.ClusterRoleBinding], error)
	EnsureRoles(ctx context.Context, resources []WSAResource) (map[string]rbacv1.Role, error)
	EnsureServiceAccounts(
		ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string,
	) error
	EnsureRoleBindings(
		ctx context.Context, resources []WSAResource, createdRoles map[string]rbacv1.Role,
		wsaToServiceAccounts map[string][]string, targetNamespaces []string,
	) error
	GarbageCollectServiceAccounts(
		ctx context.Context, expectedServiceAccounts *set.Set[string], targetNamespaces *set.Set[string],
	) (ctrl.Result, error)
}

type ResourceManagementService struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewResourceManagementService(newClient client.Client) ResourceManagementService {
	return ResourceManagementService{
		client: newClient,
		scheme: nil, // Will be set when needed
	}
}

func NewResourceManagementServiceWithScheme(newClient client.Client, scheme *runtime.Scheme) ResourceManagementService {
	return ResourceManagementService{
		client: newClient,
		scheme: scheme,
	}
}

func (r ResourceManagementService) setOwnerReference(ctx context.Context, resource WSAResource, obj client.Object) {
	if r.scheme == nil {
		return
	}

	logger := log.FromContext(ctx).WithName("setOwnerReference")
	owner := resource.GetOwnerObject()

	switch o := owner.(type) {
	case *v1beta1.WorkloadServiceAccount:
		if err := controllerutil.SetControllerReference(o, obj, r.scheme); err != nil {
			logger.Error(err, "failed to set controller reference", "objectType", fmt.Sprintf("%T", obj))
		}
	case *v1beta1.ClusterWorkloadServiceAccount:
		if err := controllerutil.SetControllerReference(o, obj, r.scheme); err != nil {
			logger.Error(err, "failed to set controller reference", "objectType", fmt.Sprintf("%T", obj))
		}
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

func (r ResourceManagementService) GetClusterWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.ClusterWorkloadServiceAccount], error) {
	cwsaList := &v1beta1.ClusterWorkloadServiceAccountList{}
	err := r.client.List(ctx, cwsaList)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster workload service accounts: %w", err)
	}

	return func(yield func(*v1beta1.ClusterWorkloadServiceAccount) bool) {
		for _, v := range cwsaList.Items {
			if !yield(&v) {
				return
			}
		}
	}, nil
}

func (r ResourceManagementService) GetServiceAccounts(ctx context.Context) (iter.Seq[*corev1.ServiceAccount], error) {
	saList := &corev1.ServiceAccountList{}
	err := r.client.List(ctx, saList)
	if err != nil {
		return nil, fmt.Errorf("failed to list service accounts: %w", err)
	}

	return func(yield func(*corev1.ServiceAccount) bool) {
		for _, v := range saList.Items {
			if IsOctopusManaged(v.Labels) {
				if !yield(&v) {
					return
				}
			}
		}
	}, nil
}

func (r ResourceManagementService) GetRoles(ctx context.Context) (iter.Seq[*rbacv1.Role], error) {
	roleList := &rbacv1.RoleList{}
	err := r.client.List(ctx, roleList)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}

	return func(yield func(*rbacv1.Role) bool) {
		for _, v := range roleList.Items {
			if IsOctopusManaged(v.Labels) {
				if !yield(&v) {
					return
				}
			}
		}
	}, nil
}

func (r ResourceManagementService) GetClusterRoles(ctx context.Context) (iter.Seq[*rbacv1.ClusterRole], error) {
	clusterRoleList := &rbacv1.ClusterRoleList{}
	err := r.client.List(ctx, clusterRoleList)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster roles: %w", err)
	}

	return func(yield func(*rbacv1.ClusterRole) bool) {
		for _, v := range clusterRoleList.Items {
			if IsOctopusManaged(v.Labels) {
				if !yield(&v) {
					return
				}
			}
		}
	}, nil
}

func (r ResourceManagementService) GetRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.RoleBinding], error) {
	roleBindingList := &rbacv1.RoleBindingList{}
	err := r.client.List(ctx, roleBindingList)
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}

	return func(yield func(*rbacv1.RoleBinding) bool) {
		for _, v := range roleBindingList.Items {
			if IsOctopusManaged(v.Labels) {
				if !yield(&v) {
					return
				}
			}
		}
	}, nil
}

func (r ResourceManagementService) GetClusterRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.ClusterRoleBinding], error) {
	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	err := r.client.List(ctx, clusterRoleBindingList)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster role bindings: %w", err)
	}

	return func(yield func(*rbacv1.ClusterRoleBinding) bool) {
		for _, v := range clusterRoleBindingList.Items {
			if IsOctopusManaged(v.Labels) {
				if !yield(&v) {
					return
				}
			}
		}
	}, nil
}

func (r ResourceManagementService) EnsureRoles(
	ctx context.Context, resources []WSAResource,
) (map[string]rbacv1.Role, error) {
	createdRoles := make(map[string]rbacv1.Role)
	var err error

	for _, resource := range resources {
		ctxWithTimeout, cancel := r.getContextWithTimeout(time.Second * 30)
		if role, createErr := r.createRoleIfNeeded(ctxWithTimeout, resource); createErr != nil {
			err = multierr.Append(err, createErr)
		} else if role.Name != "" {
			createdRoles[resource.GetName()] = role
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
	ctx context.Context, resources []WSAResource, createdRoles map[string]rbacv1.Role,
	wsaToServiceAccounts map[string][]string, targetNamespaces []string,
) error {
	logger := log.FromContext(ctx).WithName("ensureRoleBindings")
	var err error

	// Loop over resources and create role bindings for each
	for _, resource := range resources {
		serviceAccounts, exists := wsaToServiceAccounts[resource.GetName()]
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
		if bindErr := r.createRoleBindingsForResource(ctxWithTimeout, resource, allNamespacedServiceAccounts, createdRoles); bindErr != nil {
			logger.Error(bindErr, "failed to create role bindings for resource", "name", resource.GetName())
			err = multierr.Append(err, fmt.Errorf("failed to ensure role bindings for resource %s: %w", resource.GetName(), bindErr))
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

func (r ResourceManagementService) createRoleIfNeeded(ctx context.Context, resource WSAResource) (rbacv1.Role, error) {
	logger := log.FromContext(ctx).WithName("createRoleIfNeeded")

	permissions := resource.GetPermissionRules()
	if len(permissions) == 0 {
		return rbacv1.Role{}, nil
	}

	// Cluster-scoped resources create ClusterRoles not Roles
	if resource.IsClusterScoped() {
		clusterRole, err := r.createClusterRoleIfNeeded(ctx, resource)
		if err != nil {
			return rbacv1.Role{}, err
		}
		// Return a Role with just the name for compatibility with existing code
		// The name will be used to create ClusterRoleBindings in createRoleBindingsForResource
		return rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRole.Name,
			},
		}, nil
	}

	namespace := resource.GetNamespace()
	// Generate a role name based on permissions hash to ensure uniqueness
	permissionsHash := shortHash(fmt.Sprintf("%+v", permissions))
	roleName := fmt.Sprintf("octopus-role-%s", permissionsHash)

	role := rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		Rules: permissions,
	}

	r.setOwnerReference(ctx, resource, &role)

	// Use server-side apply for idempotent resource management
	// TODO: When passing ctx, client always failed with ctx cancelled errors
	err := r.client.Patch(context.TODO(), &role, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return rbacv1.Role{}, fmt.Errorf("failed to apply Role %s: %w", roleName, err)
	}

	logger.Info("Applied Role", "name", roleName, "permissions", len(permissions))
	return role, nil
}

func (r ResourceManagementService) createClusterRoleIfNeeded(
	ctx context.Context, resource WSAResource,
) (rbacv1.ClusterRole, error) {
	logger := log.FromContext(ctx).WithName("createClusterRoleIfNeeded")

	permissions := resource.GetPermissionRules()
	if len(permissions) == 0 {
		return rbacv1.ClusterRole{}, nil
	}

	// Generate a cluster role name based on permissions hash to ensure uniqueness
	permissionsHash := shortHash(fmt.Sprintf("%+v", permissions))
	clusterRoleName := fmt.Sprintf("octopus-clusterrole-%s", permissionsHash)

	clusterRole := rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		Rules: permissions,
	}

	r.setOwnerReference(ctx, resource, &clusterRole)

	// Use server-side apply for idempotent resource management
	err := r.client.Patch(context.TODO(), &clusterRole, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return rbacv1.ClusterRole{}, fmt.Errorf("failed to apply ClusterRole %s: %w", clusterRoleName, err)
	}

	logger.Info("Applied ClusterRole", "name", clusterRoleName, "permissions", len(permissions))
	return clusterRole, nil
}

// createServiceAccount deep copies a template service account, sets the namespace, and applies it using server-side apply
func (r ResourceManagementService) createServiceAccount(
	ctx context.Context, namespace string, templateServiceAccount *corev1.ServiceAccount,
) error {
	logger := log.FromContext(ctx).WithName("createServiceAccount")

	serviceAccount := templateServiceAccount.DeepCopy()
	serviceAccount.TypeMeta = metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "ServiceAccount",
	}
	serviceAccount.ObjectMeta.Namespace = namespace //nolint:staticcheck // The namespace must be set in the ObjectMeta

	// Clear ResourceVersion to ensure SSA treats this as a create-or-update
	// This is critical for Server-Side Apply to work correctly
	serviceAccount.ResourceVersion = ""

	// Add managed-by label for garbage collection tracking
	if serviceAccount.Labels == nil {
		serviceAccount.Labels = make(map[string]string)
	}
	serviceAccount.Labels[ManagedByLabel] = ManagedByValue

	// Add finalizer to track ownership across all namespaces
	// This allows us to find and clean up ServiceAccounts regardless of their namespace
	finalizers := serviceAccount.GetFinalizers()
	hasFinalizer := false
	for _, f := range finalizers {
		if f == serviceAccountFinalizer {
			hasFinalizer = true
			break
		}
	}
	if !hasFinalizer {
		serviceAccount.SetFinalizers(append(finalizers, serviceAccountFinalizer))
	}

	// Log what we're about to apply for debugging
	logger.V(1).Info("Applying ServiceAccount",
		"name", serviceAccount.Name,
		"namespace", namespace,
		"annotations", serviceAccount.Annotations,
		"hasFinalizer", len(serviceAccount.Finalizers) > 0)

	// Use server-side apply for idempotent resource management
	err := r.client.Patch(ctx, serviceAccount, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		logger.Error(err, "Failed to apply ServiceAccount",
			"name", serviceAccount.Name,
			"namespace", namespace,
			"annotations", serviceAccount.Annotations)
		return fmt.Errorf("failed to apply ServiceAccount %s in namespace %s: %w", serviceAccount.Name, namespace, err)
	}

	logger.Info("Applied ServiceAccount", "name", serviceAccount.Name, "namespace", namespace)
	return nil
}

// createRoleBindingsForResource creates all role bindings for a resource with all its service accounts as subjects
func (r ResourceManagementService) createRoleBindingsForResource(
	ctx context.Context, resource WSAResource, serviceAccounts []serviceAccountInfo,
	createdRoles map[string]rbacv1.Role,
) error {
	logger := log.FromContext(ctx).WithName("createRoleBindingsForResource")
	var err error

	// Create subjects from all service accounts for this resource
	subjects := make([]rbacv1.Subject, len(serviceAccounts))
	for i, sa := range serviceAccounts {
		subjects[i] = rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      sa.name,
			Namespace: sa.namespace,
		}
	}

	// Create role bindings for explicit roles (only for namespace-scoped resources)
	for _, roleRef := range resource.GetRoles() {
		if bindErr := r.createRoleBinding(ctx, resource, roleRef, subjects); bindErr != nil {
			err = multierr.Append(err, bindErr)
		}
	}

	// Create role binding for inline permissions (if role was created)
	if createdRole, exists := createdRoles[resource.GetName()]; exists {
		if resource.IsClusterScoped() {
			// For cluster-scoped resources with inline permissions, create ClusterRoleBinding
			roleRef := rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     createdRole.Name,
				APIGroup: "rbac.authorization.k8s.io",
			}
			if bindErr := r.createClusterRoleBinding(ctx, resource, roleRef, subjects); bindErr != nil {
				err = multierr.Append(err, bindErr)
			}
		} else {
			// For namespace-scoped resources with inline permissions, create RoleBinding
			roleRef := rbacv1.RoleRef{
				Kind:     "Role",
				Name:     createdRole.Name,
				APIGroup: "rbac.authorization.k8s.io",
			}
			if bindErr := r.createRoleBinding(ctx, resource, roleRef, subjects); bindErr != nil {
				err = multierr.Append(err, bindErr)
			}
		}
	}

	// Create bindings for cluster roles
	// For cluster-scoped resources, create ClusterRoleBindings
	// For namespace-scoped resources, create RoleBindings that reference ClusterRoles
	for _, roleRef := range resource.GetClusterRoles() {
		if resource.IsClusterScoped() {
			if bindErr := r.createClusterRoleBinding(ctx, resource, roleRef, subjects); bindErr != nil {
				err = multierr.Append(err, bindErr)
			}
		} else {
			if bindErr := r.createRoleBinding(ctx, resource, roleRef, subjects); bindErr != nil {
				err = multierr.Append(err, bindErr)
			}
		}
	}

	logger.Info("Created role bindings for resource", "name", resource.GetName(), "namespace", resource.GetNamespace(), "serviceAccounts", len(serviceAccounts))
	return err
}

func (r ResourceManagementService) createRoleBinding(
	ctx context.Context, resource WSAResource, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject,
) error {
	logger := log.FromContext(ctx).WithName("createRoleBinding")

	name := fmt.Sprintf("octopus-rb-%s", shortHash(fmt.Sprintf("%s-%s", resource.GetName(), roleRef.Name)))
	namespace := resource.GetNamespace()

	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}

	if !resource.IsClusterScoped() {
		r.setOwnerReference(ctx, resource, roleBinding)
	}

	// Use server-side apply for idempotent resource management
	err := r.client.Patch(ctx, roleBinding, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return fmt.Errorf("failed to apply RoleBinding %s in namespace %s: %w", name, namespace, err)
	}

	logger.Info("Applied RoleBinding", "name", name, "namespace", namespace, "roleRef", roleRef.Name, "resource", resource.GetName())
	return nil
}

func (r ResourceManagementService) createClusterRoleBinding(
	ctx context.Context, resource WSAResource, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject,
) error {
	logger := log.FromContext(ctx).WithName("createClusterRoleBinding")

	// ClusterRoleBindings are non-namespaced, so we use resource name to ensure uniqueness
	name := fmt.Sprintf("octopus-crb-%s", shortHash(fmt.Sprintf("%s-%s", resource.GetName(), roleRef.Name)))

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}

	if resource.IsClusterScoped() {
		r.setOwnerReference(ctx, resource, clusterRoleBinding)
	}

	// Use server-side apply for idempotent resource management
	err := r.client.Patch(ctx, clusterRoleBinding, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return fmt.Errorf("failed to apply ClusterRoleBinding %s: %w", name, err)
	}

	logger.Info("Applied ClusterRoleBinding", "name", name, "roleRef", roleRef.Name, "resource", resource.GetName())
	return nil
}

// GarbageCollectServiceAccounts deletes ServiceAccounts that are managed by this controller
// but are no longer needed (not in the expectedServiceAccounts set or in out-of-scope namespaces).
// Returns ctrl.Result with RequeueAfter set when ServiceAccounts are still in use by pods.
func (r ResourceManagementService) GarbageCollectServiceAccounts(
	ctx context.Context, expectedServiceAccounts *set.Set[string], targetNamespaces *set.Set[string],
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("garbageCollectServiceAccounts")

	saList := &corev1.ServiceAccountList{}
	if listErr := r.client.List(ctx, saList, client.MatchingLabels{ManagedByLabel: ManagedByValue}); listErr != nil {
		logger.Error(listErr, "failed to list ServiceAccounts")
		return ctrl.Result{}, fmt.Errorf("failed to list ServiceAccounts: %w", listErr)
	}

	var needsRequeue bool
	for i := range saList.Items {
		sa := &saList.Items[i]

		if !r.shouldDeleteServiceAccount(sa, expectedServiceAccounts, targetNamespaces) {
			logger.V(1).Info("Keeping ServiceAccount",
				"name", sa.Name,
				"namespace", sa.Namespace,
				"annotations", sa.Annotations,
				"reason", "still needed")
			continue
		}

		if sa.DeletionTimestamp.IsZero() {
			if err := r.markServiceAccountForDeletion(ctx, sa, targetNamespaces); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			requeue, err := r.completeServiceAccountDeletion(ctx, sa, targetNamespaces)
			if err != nil {
				return ctrl.Result{}, err
			}
			if requeue {
				needsRequeue = true
			}
		}
	}

	if needsRequeue {
		logger.Info("Some ServiceAccounts still in use by pods, requeueing in 5s")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r ResourceManagementService) shouldDeleteServiceAccount(
	sa *corev1.ServiceAccount,
	expectedServiceAccounts *set.Set[string],
	targetNamespaces *set.Set[string],
) bool {
	namespaceInScope := targetNamespaces.Contains(sa.Namespace)
	nameExpected := expectedServiceAccounts.Contains(sa.Name)
	return !namespaceInScope || !nameExpected
}

func (r ResourceManagementService) markServiceAccountForDeletion(
	ctx context.Context,
	sa *corev1.ServiceAccount,
	targetNamespaces *set.Set[string],
) error {
	logger := log.FromContext(ctx).WithName("markServiceAccountForDeletion")

	namespaceInScope := targetNamespaces.Contains(sa.Namespace)
	logger.Info("Marking ServiceAccount for deletion",
		"name", sa.Name,
		"namespace", sa.Namespace,
		"reason", fmt.Sprintf("namespaceInScope=%v", namespaceInScope),
		"annotations", sa.Annotations)

	if deleteErr := r.client.Delete(ctx, sa); deleteErr != nil {
		if !errors.IsNotFound(deleteErr) {
			logger.Error(deleteErr, "failed to initiate deletion of ServiceAccount",
				"name", sa.Name, "namespace", sa.Namespace)
			return fmt.Errorf("failed to initiate deletion of ServiceAccount %s in namespace %s: %w",
				sa.Name, sa.Namespace, deleteErr)
		}
	} else {
		logger.Info("Initiated deletion of ServiceAccount (will check for active pods on next reconcile)",
			"name", sa.Name, "namespace", sa.Namespace)
	}
	return nil
}

func (r ResourceManagementService) completeServiceAccountDeletion(
	ctx context.Context,
	sa *corev1.ServiceAccount,
	targetNamespaces *set.Set[string],
) (bool, error) {
	logger := log.FromContext(ctx).WithName("completeServiceAccountDeletion")

	logger.Info("ServiceAccount marked for deletion, checking for active pods",
		"name", sa.Name,
		"namespace", sa.Namespace)

	inUse, pods, checkErr := r.IsServiceAccountInUse(ctx, sa.Name, targetNamespaces)
	if checkErr != nil {
		logger.Error(checkErr, "failed to check if ServiceAccount is in use",
			"name", sa.Name, "namespace", sa.Namespace)
		return false, fmt.Errorf("failed to check if ServiceAccount %s is in use: %w", sa.Name, checkErr)
	}

	if inUse {
		logger.Info("Deferring ServiceAccount deletion due to active pods, will requeue in 30s",
			"name", sa.Name,
			"namespace", sa.Namespace,
			"activePods", pods,
			"podCount", len(pods))
		return true, nil
	}

	logger.Info("No active pods found, completing ServiceAccount deletion",
		"name", sa.Name,
		"namespace", sa.Namespace)

	return false, r.removeServiceAccountFinalizer(ctx, sa)
}

func (r ResourceManagementService) removeServiceAccountFinalizer(
	ctx context.Context,
	sa *corev1.ServiceAccount,
) error {
	logger := log.FromContext(ctx).WithName("removeServiceAccountFinalizer")

	hasFinalizer := false
	for _, f := range sa.GetFinalizers() {
		if f == serviceAccountFinalizer {
			hasFinalizer = true
			break
		}
	}

	if !hasFinalizer {
		return nil
	}

	finalizers := sa.GetFinalizers()
	newFinalizers := []string{}
	for _, f := range finalizers {
		if f != serviceAccountFinalizer {
			newFinalizers = append(newFinalizers, f)
		}
	}
	sa.SetFinalizers(newFinalizers)

	if updateErr := r.client.Update(ctx, sa); updateErr != nil {
		if !errors.IsNotFound(updateErr) {
			logger.Error(updateErr, "failed to remove finalizer from ServiceAccount",
				"name", sa.Name, "namespace", sa.Namespace)
			return fmt.Errorf("failed to remove finalizer from ServiceAccount %s in namespace %s: %w",
				sa.Name, sa.Namespace, updateErr)
		}
	} else {
		logger.Info("Removed finalizer from ServiceAccount, deletion will complete",
			"name", sa.Name, "namespace", sa.Namespace)
	}
	return nil
}

// IsServiceAccountInUse checks if any pods in the target namespaces are currently using the specified ServiceAccount.
// It returns true if any pods in Running, Pending, or Terminating state are using the SA,
// along with the names of those pods for logging purposes.
func (r ResourceManagementService) IsServiceAccountInUse(
	ctx context.Context, saName string, targetNamespaces *set.Set[string],
) (bool, []string, error) {
	logger := log.FromContext(ctx).WithName("isServiceAccountInUse")
	var activePods []string

	// Check each target namespace for pods using this ServiceAccount
	for namespace := range targetNamespaces.Items() {
		rawPodList := &corev1.PodList{}
		err := r.client.List(ctx, rawPodList,
			client.InNamespace(namespace))

		if err != nil {
			logger.Error(err, "failed to list pods in namespace", "namespace", namespace, "serviceAccount", saName)
			return false, nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
		}

		// Filter pods that are using the specified ServiceAccount
		podList := &corev1.PodList{}
		for i := range rawPodList.Items {
			pod := &rawPodList.Items[i]
			if pod.Spec.ServiceAccountName == saName {
				podList.Items = append(podList.Items, *pod)
			}
		}

		// Check if any pods are in Running or Pending state
		for i := range podList.Items {
			pod := &podList.Items[i]

			switch pod.Status.Phase {
			case corev1.PodRunning, corev1.PodPending:
				activePods = append(activePods, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}
		}
	}

	if len(activePods) > 0 {
		logger.V(1).Info("ServiceAccount is in use by active pods",
			"serviceAccount", saName,
			"podCount", len(activePods),
			"pods", activePods)
		return true, activePods, nil
	}

	return false, nil, nil
}
