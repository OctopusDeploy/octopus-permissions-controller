package rules

import (
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
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
