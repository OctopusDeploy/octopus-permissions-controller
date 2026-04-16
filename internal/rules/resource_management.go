package rules

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	EnsureRoles(ctx context.Context, resources []WSAResource) (map[types.NamespacedName]rbacv1.Role, error)
	EnsureServiceAccounts(
		ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string,
	) error
	EnsureRoleBindings(
		ctx context.Context, resources []WSAResource, createdRoles map[types.NamespacedName]rbacv1.Role,
		wsaToServiceAccounts map[types.NamespacedName][]string, targetNamespaces []string,
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

func (r ResourceManagementService) ownerReferenceAC(resource WSAResource) *metav1ac.OwnerReferenceApplyConfiguration {
	if r.scheme == nil {
		return nil
	}

	owner := resource.GetOwnerObject()

	switch o := owner.(type) {
	case *v1beta1.WorkloadServiceAccount:
		return metav1ac.OwnerReference().
			WithAPIVersion(v1beta1.GroupVersion.String()).
			WithKind("WorkloadServiceAccount").
			WithName(o.Name).
			WithUID(o.UID).
			WithController(true).
			WithBlockOwnerDeletion(true)
	case *v1beta1.ClusterWorkloadServiceAccount:
		return metav1ac.OwnerReference().
			WithAPIVersion(v1beta1.GroupVersion.String()).
			WithKind("ClusterWorkloadServiceAccount").
			WithName(o.Name).
			WithUID(o.UID).
			WithController(true).
			WithBlockOwnerDeletion(true)
	}

	return nil
}

func roleRefAC(ref rbacv1.RoleRef) *rbacv1ac.RoleRefApplyConfiguration {
	return rbacv1ac.RoleRef().
		WithAPIGroup(ref.APIGroup).
		WithKind(ref.Kind).
		WithName(ref.Name)
}

func subjectsAC(subjects []rbacv1.Subject) []*rbacv1ac.SubjectApplyConfiguration {
	result := make([]*rbacv1ac.SubjectApplyConfiguration, 0, len(subjects))
	for _, s := range subjects {
		result = append(result, rbacv1ac.Subject().
			WithKind(s.Kind).
			WithName(s.Name).
			WithNamespace(s.Namespace))
	}
	return result
}

func policyRulesAC(rules []rbacv1.PolicyRule) []*rbacv1ac.PolicyRuleApplyConfiguration {
	result := make([]*rbacv1ac.PolicyRuleApplyConfiguration, 0, len(rules))
	for _, rule := range rules {
		result = append(result, rbacv1ac.PolicyRule().
			WithAPIGroups(rule.APIGroups...).
			WithResources(rule.Resources...).
			WithResourceNames(rule.ResourceNames...).
			WithVerbs(rule.Verbs...).
			WithNonResourceURLs(rule.NonResourceURLs...))
	}
	return result
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
) (map[types.NamespacedName]rbacv1.Role, error) {
	createdRoles := make(map[types.NamespacedName]rbacv1.Role)
	var err error

	for _, resource := range resources {
		ctxWithTimeout, cancel := r.getContextWithTimeout(ctx, time.Second*30)
		if role, createErr := r.createRoleIfNeeded(ctxWithTimeout, resource); createErr != nil {
			err = multierr.Append(err, createErr)
		} else if role.Name != "" {
			createdRoles[resource.GetNamespacedName()] = role
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
	ctx context.Context, resources []WSAResource, createdRoles map[types.NamespacedName]rbacv1.Role,
	wsaToServiceAccounts map[types.NamespacedName][]string, targetNamespaces []string,
) error {
	logger := log.FromContext(ctx).WithName("ensureRoleBindings")
	var err error

	// Loop over resources and create role bindings for each
	for _, resource := range resources {
		serviceAccounts, exists := wsaToServiceAccounts[resource.GetNamespacedName()]
		if !exists || len(serviceAccounts) == 0 {
			continue
		}

		allNamespacedServiceAccounts := make([]serviceAccountInfo, 0, len(targetNamespaces)*len(serviceAccounts))
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
			logger.Error(bindErr, "failed to create role bindings for resource", "resource", resource.GetNamespacedName().String())
			err = multierr.Append(err, fmt.Errorf("failed to ensure role bindings for resource %s: %w", resource.GetNamespacedName().String(), bindErr))
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

	roleAC := rbacv1ac.Role(name, namespace).
		WithLabels(map[string]string{
			ManagedByLabel: ManagedByValue,
		}).
		WithRules(policyRulesAC(permissions)...)

	if ownerRef := r.ownerReferenceAC(resource); ownerRef != nil {
		roleAC.WithOwnerReferences(ownerRef)
	}

	err := r.client.Apply(ctx, roleAC,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return rbacv1.Role{}, fmt.Errorf("failed to apply Role %s: %w", name, err)
	}

	logger.Info("Applied Role", "name", name, "permissions", len(permissions))
	return rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
			},
		},
		Rules: permissions,
	}, nil
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

	clusterRoleAC := rbacv1ac.ClusterRole(name).
		WithLabels(map[string]string{
			ManagedByLabel: ManagedByValue,
		}).
		WithRules(policyRulesAC(permissions)...)

	if ownerRef := r.ownerReferenceAC(resource); ownerRef != nil {
		clusterRoleAC.WithOwnerReferences(ownerRef)
	}

	err := r.client.Apply(ctx, clusterRoleAC,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return rbacv1.ClusterRole{}, fmt.Errorf("failed to apply ClusterRole %s: %w", name, err)
	}

	logger.Info("Applied ClusterRole", "name", name, "permissions", len(permissions))
	return rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, nil
}

// createServiceAccount builds an apply configuration from a template service account, sets the namespace, and applies it using server-side apply
func (r ResourceManagementService) createServiceAccount(
	ctx context.Context, namespace string, templateServiceAccount *corev1.ServiceAccount,
) error {
	logger := log.FromContext(ctx).WithName("createServiceAccount")

	// Build labels from template, adding managed-by label
	labels := make(map[string]string)
	maps.Copy(labels, templateServiceAccount.Labels)
	labels[ManagedByLabel] = ManagedByValue

	saAC := corev1ac.ServiceAccount(templateServiceAccount.Name, namespace).
		WithLabels(labels)

	if len(templateServiceAccount.Annotations) > 0 {
		saAC.WithAnnotations(templateServiceAccount.Annotations)
	}

	// Log what we're about to apply for debugging
	logger.V(1).Info("Applying ServiceAccount",
		"name", templateServiceAccount.Name,
		"namespace", namespace,
		"annotations", templateServiceAccount.Annotations)

	// Use server-side apply for idempotent resource management
	err := r.client.Apply(ctx, saAC,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		logger.Error(err, "Failed to apply ServiceAccount",
			"name", templateServiceAccount.Name,
			"namespace", namespace,
			"annotations", templateServiceAccount.Annotations)
		return fmt.Errorf("failed to apply ServiceAccount %s in namespace %s: %w", templateServiceAccount.Name, namespace, err)
	}

	logger.Info("Applied ServiceAccount", "name", templateServiceAccount.Name, "namespace", namespace)
	return nil
}

// createRoleBindingsForResource creates all role bindings for a resource with all its service accounts as subjects
func (r ResourceManagementService) createRoleBindingsForResource(
	ctx context.Context, resource WSAResource, serviceAccounts []serviceAccountInfo,
	createdRoles map[types.NamespacedName]rbacv1.Role,
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
	if createdRole, exists := createdRoles[resource.GetNamespacedName()]; exists {
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

	logger.Info("Created role bindings for resource", "resource", resource.GetNamespacedName().String(), "serviceAccounts", len(serviceAccounts))
	return err
}

func (r ResourceManagementService) createRoleBinding(
	ctx context.Context, resource WSAResource, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject,
) error {
	logger := log.FromContext(ctx).WithName("createRoleBinding")

	name := roleBindingName(resource.GetName(), roleRef.Name)
	namespace := resource.GetNamespace()

	rbAC := rbacv1ac.RoleBinding(name, namespace).
		WithLabels(map[string]string{
			ManagedByLabel: ManagedByValue,
		}).
		WithRoleRef(roleRefAC(roleRef)).
		WithSubjects(subjectsAC(subjects)...)

	if !resource.IsClusterScoped() {
		if ownerRef := r.ownerReferenceAC(resource); ownerRef != nil {
			rbAC.WithOwnerReferences(ownerRef)
		}
	}

	err := r.client.Apply(ctx, rbAC,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return fmt.Errorf("failed to apply RoleBinding %s in namespace %s: %w", name, namespace, err)
	}

	logger.Info("Applied RoleBinding", "name", name, "namespace", namespace, "roleRef", roleRef.Name, "resource", resource.GetNamespacedName().String())
	return nil
}

func (r ResourceManagementService) createClusterRoleBinding(
	ctx context.Context, resource WSAResource, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject,
) error {
	logger := log.FromContext(ctx).WithName("createClusterRoleBinding")

	name := clusterRoleBindingName(resource.GetName(), roleRef.Name)

	crbAC := rbacv1ac.ClusterRoleBinding(name).
		WithLabels(map[string]string{
			ManagedByLabel: ManagedByValue,
		}).
		WithRoleRef(roleRefAC(roleRef)).
		WithSubjects(subjectsAC(subjects)...)

	if resource.IsClusterScoped() {
		if ownerRef := r.ownerReferenceAC(resource); ownerRef != nil {
			crbAC.WithOwnerReferences(ownerRef)
		}
	}

	err := r.client.Apply(ctx, crbAC,
		client.ForceOwnership,
		client.FieldOwner("octopus-permissions-controller"))
	if err != nil {
		return fmt.Errorf("failed to apply ClusterRoleBinding %s: %w", name, err)
	}

	logger.Info("Applied ClusterRoleBinding", "name", name, "roleRef", roleRef.Name, "resource", resource.GetNamespacedName().String())
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

	expectedBindings := set.New[types.NamespacedName](len(resources) * len(targetNamespaces))
	for _, resource := range resources {
		if resource.IsClusterScoped() {
			continue
		}

		permissions := resource.GetPermissionRules()
		if len(permissions) > 0 {
			bindingKey := types.NamespacedName{
				Namespace: resource.GetNamespace(),
				Name:      roleBindingName(resource.GetName(), roleName(permissions)),
			}
			expectedBindings.Insert(bindingKey)
		}

		allRoleRefs := append(resource.GetRoles(), resource.GetClusterRoles()...)
		for _, roleRef := range allRoleRefs {
			bindingKey := types.NamespacedName{
				Namespace: resource.GetNamespace(),
				Name:      roleBindingName(resource.GetName(), roleRef.Name),
			}
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
		bindingKey := types.NamespacedName{
			Namespace: rb.Namespace,
			Name:      rb.Name,
		}
		if !expectedBindings.Contains(bindingKey) {
			logger.Info("Deleting orphaned RoleBinding",
				"roleBinding", bindingKey.String())
			r.markForDeletion(rb)
			if deleteErr := r.client.Delete(ctx, rb); deleteErr != nil && !errors.IsNotFound(deleteErr) {
				deleteErrors = multierr.Append(deleteErrors, fmt.Errorf("failed to delete RoleBinding %s: %w",
					bindingKey, deleteErr))
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

	// Since ClusterRoleBindings are global, we only need to track by name, not NamespacedName
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
