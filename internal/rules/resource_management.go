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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/hashicorp/go-set/v3"
)

// GCTrackerInterface defines the interface for tracking resources being garbage collected.
type GCTrackerInterface interface {
	MarkForDeletion(uid types.UID)
}

// serviceAccountInfo tracks a service account name and namespace
type serviceAccountInfo struct {
	name      string
	namespace string
}

func roleName(permissions []rbacv1.PolicyRule) string {
	return fmt.Sprintf("octopus-role-%s", hashPermissions(permissions))
}

func clusterRoleName(permissions []rbacv1.PolicyRule) string {
	return fmt.Sprintf("octopus-clusterrole-%s", hashPermissions(permissions))
}

func roleBindingName(resourceName, roleRefName string) string {
	return fmt.Sprintf("octopus-rb-%s", shortHash(fmt.Sprintf("%s-%s", resourceName, roleRefName)))
}

func clusterRoleBindingName(resourceName, roleRefName string) string {
	return fmt.Sprintf("octopus-crb-%s", shortHash(fmt.Sprintf("%s-%s", resourceName, roleRefName)))
}

func hashPermissions(permissions []rbacv1.PolicyRule) string {
	return shortHash(fmt.Sprintf("%+v", permissions))
}

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
	GarbageCollectRoles(ctx context.Context, resources []WSAResource) error
	GarbageCollectRoleBindings(ctx context.Context, resources []WSAResource, targetNamespaces []string) error
	GarbageCollectClusterRoleBindings(ctx context.Context, resources []WSAResource) error
}

type ResourceManagementService struct {
	client    client.Client
	scheme    *runtime.Scheme
	gcTracker GCTrackerInterface
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

// SetGCTracker sets the garbage collection tracker for filtering delete events.
func (r *ResourceManagementService) SetGCTracker(tracker GCTrackerInterface) {
	r.gcTracker = tracker
}

func (r ResourceManagementService) markForDeletion(obj client.Object) {
	if r.gcTracker != nil {
		r.gcTracker.MarkForDeletion(obj.GetUID())
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
		ctxWithTimeout, cancel := r.getContextWithTimeout(ctx, time.Second*30)
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
			ctxWithTimeout, cancel := r.getContextWithTimeout(ctx, time.Second*30)
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

		ctxWithTimeout, cancel := r.getContextWithTimeout(ctx, time.Second*30)
		if bindErr := r.createRoleBindingsForResource(ctxWithTimeout, resource, allNamespacedServiceAccounts, createdRoles); bindErr != nil {
			logger.Error(bindErr, "failed to create role bindings for resource", "name", resource.GetName())
			err = multierr.Append(err, fmt.Errorf("failed to ensure role bindings for resource %s: %w", resource.GetName(), bindErr))
		}
		cancel()
	}

	return err
}

// Helper methods for ResourceManagementService

func (r ResourceManagementService) getContextWithTimeout(
	ctx context.Context, timeout time.Duration,
) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
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
	name := roleName(permissions)

	role := rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		Rules: permissions,
	}

	r.setOwnerReference(ctx, resource, &role)

	err := r.client.Patch(ctx, &role, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return rbacv1.Role{}, fmt.Errorf("failed to apply Role %s: %w", name, err)
	}

	logger.Info("Applied Role", "name", name, "permissions", len(permissions))
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

	name := clusterRoleName(permissions)

	clusterRole := rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		Rules: permissions,
	}

	r.setOwnerReference(ctx, resource, &clusterRole)

	err := r.client.Patch(ctx, &clusterRole, client.Apply,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return rbacv1.ClusterRole{}, fmt.Errorf("failed to apply ClusterRole %s: %w", name, err)
	}

	logger.Info("Applied ClusterRole", "name", name, "permissions", len(permissions))
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

	// Log what we're about to apply for debugging
	logger.V(1).Info("Applying ServiceAccount",
		"name", serviceAccount.Name,
		"namespace", namespace,
		"annotations", serviceAccount.Annotations)

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

	name := roleBindingName(resource.GetName(), roleRef.Name)
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

	name := clusterRoleBindingName(resource.GetName(), roleRef.Name)

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
func (r ResourceManagementService) GarbageCollectServiceAccounts(
	ctx context.Context, expectedServiceAccounts *set.Set[string], targetNamespaces *set.Set[string],
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("garbageCollectServiceAccounts")

	saList := &corev1.ServiceAccountList{}
	if listErr := r.client.List(ctx, saList, client.MatchingLabels{ManagedByLabel: ManagedByValue}); listErr != nil {
		logger.Error(listErr, "failed to list ServiceAccounts")
		return ctrl.Result{}, fmt.Errorf("failed to list ServiceAccounts: %w", listErr)
	}

	var deleteErrors error
	deletedCount := 0
	for i := range saList.Items {
		sa := &saList.Items[i]

		if !r.shouldDeleteServiceAccount(sa, expectedServiceAccounts, targetNamespaces) {
			continue
		}

		logger.Info("Deleting orphaned ServiceAccount",
			"name", sa.Name,
			"namespace", sa.Namespace)
		r.markForDeletion(sa)
		if deleteErr := r.client.Delete(ctx, sa); deleteErr != nil && !errors.IsNotFound(deleteErr) {
			deleteErrors = multierr.Append(deleteErrors, fmt.Errorf("failed to delete ServiceAccount %s/%s: %w",
				sa.Namespace, sa.Name, deleteErr))
		} else {
			deletedCount++
		}
	}

	if deletedCount > 0 {
		logger.Info("Garbage collected ServiceAccounts", "deletedCount", deletedCount)
	}

	return ctrl.Result{}, deleteErrors
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

func (r ResourceManagementService) GarbageCollectRoles(ctx context.Context, resources []WSAResource) error {
	logger := log.FromContext(ctx).WithName("garbageCollectRoles")

	expectedRoles := set.New[string](len(resources))
	for _, resource := range resources {
		permissions := resource.GetPermissionRules()
		if len(permissions) == 0 {
			continue
		}

		if resource.IsClusterScoped() {
			expectedRoles.Insert(clusterRoleName(permissions))
		} else {
			expectedRoles.Insert(roleName(permissions))
		}
	}

	roleIter, err := r.GetRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Roles: %w", err)
	}

	var deleteErrors error
	deletedCount := 0
	for role := range roleIter {
		if !expectedRoles.Contains(role.Name) {
			logger.Info("Deleting orphaned Role",
				"name", role.Name,
				"namespace", role.Namespace)
			r.markForDeletion(role)
			if deleteErr := r.client.Delete(ctx, role); deleteErr != nil && !errors.IsNotFound(deleteErr) {
				deleteErrors = multierr.Append(deleteErrors, fmt.Errorf("failed to delete Role %s/%s: %w",
					role.Namespace, role.Name, deleteErr))
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		logger.Info("Garbage collected Roles", "deletedCount", deletedCount)
	}

	return deleteErrors
}

func (r ResourceManagementService) GarbageCollectRoleBindings(
	ctx context.Context, resources []WSAResource, targetNamespaces []string,
) error {
	logger := log.FromContext(ctx).WithName("garbageCollectRoleBindings")

	expectedBindings := set.New[string](len(resources) * len(targetNamespaces))
	for _, resource := range resources {
		if resource.IsClusterScoped() {
			continue
		}

		permissions := resource.GetPermissionRules()
		if len(permissions) > 0 {
			bindingKey := fmt.Sprintf("%s/%s", resource.GetNamespace(), roleBindingName(resource.GetName(), roleName(permissions)))
			expectedBindings.Insert(bindingKey)
		}

		allRoleRefs := append(resource.GetRoles(), resource.GetClusterRoles()...)
		for _, roleRef := range allRoleRefs {
			bindingKey := fmt.Sprintf("%s/%s", resource.GetNamespace(), roleBindingName(resource.GetName(), roleRef.Name))
			expectedBindings.Insert(bindingKey)
		}

	}

	rbIter, err := r.GetRoleBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list RoleBindings: %w", err)
	}

	var deleteErrors error
	deletedCount := 0
	for rb := range rbIter {
		bindingKey := fmt.Sprintf("%s/%s", rb.Namespace, rb.Name)
		if !expectedBindings.Contains(bindingKey) {
			logger.Info("Deleting orphaned RoleBinding",
				"name", rb.Name,
				"namespace", rb.Namespace)
			r.markForDeletion(rb)
			if deleteErr := r.client.Delete(ctx, rb); deleteErr != nil && !errors.IsNotFound(deleteErr) {
				deleteErrors = multierr.Append(deleteErrors, fmt.Errorf("failed to delete RoleBinding %s/%s: %w",
					rb.Namespace, rb.Name, deleteErr))
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		logger.Info("Garbage collected RoleBindings", "deletedCount", deletedCount)
	}

	return deleteErrors
}

func (r ResourceManagementService) GarbageCollectClusterRoleBindings(
	ctx context.Context, resources []WSAResource,
) error {
	logger := log.FromContext(ctx).WithName("garbageCollectClusterRoleBindings")

	expectedBindings := set.New[string](len(resources))
	for _, resource := range resources {
		permissions := resource.GetPermissionRules()

		if len(permissions) > 0 {
			var rn string
			if resource.IsClusterScoped() {
				rn = clusterRoleName(permissions)
			} else {
				rn = roleName(permissions)
			}
			expectedBindings.Insert(clusterRoleBindingName(resource.GetName(), rn))
		}

		for _, roleRef := range resource.GetClusterRoles() {
			expectedBindings.Insert(clusterRoleBindingName(resource.GetName(), roleRef.Name))
		}
	}

	crbIter, err := r.GetClusterRoleBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list ClusterRoleBindings: %w", err)
	}

	var deleteErrors error
	deletedCount := 0
	for crb := range crbIter {
		if !expectedBindings.Contains(crb.Name) {
			logger.Info("Deleting orphaned ClusterRoleBinding", "name", crb.Name)
			r.markForDeletion(crb)
			if deleteErr := r.client.Delete(ctx, crb); deleteErr != nil && !errors.IsNotFound(deleteErr) {
				deleteErrors = multierr.Append(deleteErrors, fmt.Errorf("failed to delete ClusterRoleBinding %s: %w",
					crb.Name, deleteErr))
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		logger.Info("Garbage collected ClusterRoleBindings", "deletedCount", deletedCount)
	}

	return deleteErrors
}
