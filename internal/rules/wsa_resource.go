package rules

import (
	"context"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/condition"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WSAResource is an internal interface that abstracts over both WorkloadServiceAccount
// and ClusterWorkloadServiceAccount to allow unified processing
type WSAResource interface {
	// GetName returns the resource name
	GetName() string

	// GetNamespace returns the namespace (empty string for cluster-scoped resources)
	GetNamespace() string

	// GetNamespacedName returns the NamespacedName of the resource
	GetNamespacedName() types.NamespacedName

	// GetScope returns the scope configuration
	GetScope() v1beta1.WorkloadServiceAccountScope

	// GetPermissionRules returns inline permission rules
	GetPermissionRules() []rbacv1.PolicyRule

	// GetRoles returns role references (only for namespace-scoped WSA)
	GetRoles() []rbacv1.RoleRef

	// GetClusterRoles returns cluster role references
	GetClusterRoles() []rbacv1.RoleRef

	// IsClusterScoped returns true if this is a cluster-scoped resource
	IsClusterScoped() bool

	// GetOwnerObject returns the underlying WSA or CWSA object for owner references
	GetOwnerObject() interface{}

	// UpdateCondition applies a status condition using SSA and updates the in-memory resource
	UpdateCondition(
		ctx context.Context, c client.Client, conditionType string, status metav1.ConditionStatus,
		reason, message string,
	) error
}

// wsaAdapter wraps a WorkloadServiceAccount to implement WSAResource
type wsaAdapter struct {
	wsa *v1beta1.WorkloadServiceAccount
}

// NewWSAResource creates a WSAResource from a WorkloadServiceAccount
func NewWSAResource(wsa *v1beta1.WorkloadServiceAccount) WSAResource {
	return &wsaAdapter{wsa: wsa}
}

func (w *wsaAdapter) GetName() string {
	return w.wsa.Name
}

func (w *wsaAdapter) GetNamespace() string {
	return w.wsa.Namespace
}

func (w *wsaAdapter) GetNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      w.wsa.Name,
		Namespace: w.wsa.Namespace,
	}
}

func (w *wsaAdapter) GetScope() v1beta1.WorkloadServiceAccountScope {
	return w.wsa.Spec.Scope
}

func (w *wsaAdapter) GetPermissionRules() []rbacv1.PolicyRule {
	return w.wsa.Spec.Permissions.Permissions
}

func (w *wsaAdapter) GetRoles() []rbacv1.RoleRef {
	return w.wsa.Spec.Permissions.Roles
}

func (w *wsaAdapter) GetClusterRoles() []rbacv1.RoleRef {
	return w.wsa.Spec.Permissions.ClusterRoles
}

func (w *wsaAdapter) IsClusterScoped() bool {
	return false
}

func (w *wsaAdapter) GetOwnerObject() interface{} {
	return w.wsa
}

func (w *wsaAdapter) GetConditions() []metav1.Condition {
	return w.wsa.Status.Conditions
}

func (w *wsaAdapter) SetConditions(conditions []metav1.Condition) {
	w.wsa.Status.Conditions = conditions
}

func (w *wsaAdapter) SetResourceVersion(rv string) {
	w.wsa.ResourceVersion = rv
}

func (w *wsaAdapter) GetAPIVersion() string {
	return v1beta1.GroupVersion.String()
}

func (w *wsaAdapter) GetKind() string {
	return "WorkloadServiceAccount"
}

func (w *wsaAdapter) GetObject() *v1beta1.WorkloadServiceAccount {
	return w.wsa
}

func (w *wsaAdapter) UpdateCondition(
	ctx context.Context, c client.Client, conditionType string, status metav1.ConditionStatus, reason, message string,
) error {
	return condition.Apply[*v1beta1.WorkloadServiceAccount](ctx, c, w, conditionType, status, reason, message)
}

// clusterWSAAdapter wraps a ClusterWorkloadServiceAccount to implement WSAResource
type clusterWSAAdapter struct {
	cwsa *v1beta1.ClusterWorkloadServiceAccount
}

// NewClusterWSAResource creates a WSAResource from a ClusterWorkloadServiceAccount
func NewClusterWSAResource(cwsa *v1beta1.ClusterWorkloadServiceAccount) WSAResource {
	return &clusterWSAAdapter{cwsa: cwsa}
}

func (c *clusterWSAAdapter) GetName() string {
	return c.cwsa.Name
}

func (c *clusterWSAAdapter) GetNamespace() string {
	// Cluster-scoped resources don't have a namespace
	return ""
}

func (c *clusterWSAAdapter) GetNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      c.cwsa.Name,
		Namespace: "",
	}
}

func (c *clusterWSAAdapter) GetScope() v1beta1.WorkloadServiceAccountScope {
	return c.cwsa.Spec.Scope
}

func (c *clusterWSAAdapter) GetPermissionRules() []rbacv1.PolicyRule {
	return c.cwsa.Spec.Permissions.Permissions
}

func (c *clusterWSAAdapter) GetRoles() []rbacv1.RoleRef {
	// ClusterWorkloadServiceAccount doesn't support namespace-scoped Roles
	return nil
}

func (c *clusterWSAAdapter) GetClusterRoles() []rbacv1.RoleRef {
	return c.cwsa.Spec.Permissions.ClusterRoles
}

func (c *clusterWSAAdapter) IsClusterScoped() bool {
	return true
}

func (c *clusterWSAAdapter) GetOwnerObject() interface{} {
	return c.cwsa
}

func (c *clusterWSAAdapter) GetConditions() []metav1.Condition {
	return c.cwsa.Status.Conditions
}

func (c *clusterWSAAdapter) SetConditions(conditions []metav1.Condition) {
	c.cwsa.Status.Conditions = conditions
}

func (c *clusterWSAAdapter) SetResourceVersion(rv string) {
	c.cwsa.ResourceVersion = rv
}

func (c *clusterWSAAdapter) GetAPIVersion() string {
	return v1beta1.GroupVersion.String()
}

func (c *clusterWSAAdapter) GetKind() string {
	return "ClusterWorkloadServiceAccount"
}

func (c *clusterWSAAdapter) GetObject() *v1beta1.ClusterWorkloadServiceAccount {
	return c.cwsa
}

func (c *clusterWSAAdapter) UpdateCondition(
	ctx context.Context, cl client.Client, conditionType string, status metav1.ConditionStatus, reason, message string,
) error {
	return condition.Apply[*v1beta1.ClusterWorkloadServiceAccount](ctx, cl, c, conditionType, status, reason, message)
}
