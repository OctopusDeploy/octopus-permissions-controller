package rules

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var discoveryLog = ctrl.Log.WithName("namespace_discovery")

// DiscoverTargetNamespaces finds namespaces containing Octopus Tentacle deployments
// these namespaces are used for creating the service accounts for use by pods deployed by Tentacle those namespaces
func DiscoverTargetNamespaces(k8sClient client.Client) ([]string, error) {
	ctx := context.Background()

	var deployments appsv1.DeploymentList
	err := k8sClient.List(ctx, &deployments, client.MatchingLabels{
		"app.kubernetes.io/name": "octopus-agent",
	})
	if err != nil {
		discoveryLog.Error(err, "Failed to list deployments with octopus agent label")
		// TODO: Consider if we want to fail without any k8s agents?
	}

	namespaceSet := make(map[string]struct{})
	for _, deployment := range deployments.Items {
		namespaceSet[deployment.Namespace] = struct{}{}
	}

	namespaces := make([]string, 0, len(namespaceSet))
	for namespace := range namespaceSet {
		namespaces = append(namespaces, namespace)
	}

	// Use default namespace as fallback for local testing if no tentacle deployments found
	if len(namespaces) == 0 {
		namespaces = []string{"default"}
		discoveryLog.Info("No octopus tentacle deployments found, using default namespace as fallback")
	} else {
		discoveryLog.Info("Discovered target namespaces for service account creation",
			"count", len(namespaces),
			"namespaces", namespaces)
	}

	return namespaces, nil
}
