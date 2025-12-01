package rules

import (
	"context"
	"regexp"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NamespaceDiscovery interface {
	DiscoverTargetNamespaces(ctx context.Context, k8sClient client.Client) ([]string, error)
	GetTargetNamespaces() []string
}

type NamespaceDiscoveryService struct {
	TargetNamespaceRegex *regexp.Regexp
}

func (nds NamespaceDiscoveryService) DiscoverTargetNamespaces(
	ctx context.Context, k8sClient client.Client,
) ([]string, error) {
	discoveryLog := log.FromContext(ctx, "component", "namespace-discovery")
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
		if nds.TargetNamespaceRegex.MatchString(namespace) {
			namespaces = append(namespaces, namespace)
		} else {
			discoveryLog.Info("Discovered namespace, but did not match TARGET_NAMESPACE_REGEX", "namespace", namespace, "regex", nds.TargetNamespaceRegex.String())
		}
	}

	if len(namespaces) == 0 {
		discoveryLog.Info("No octopus tentacle deployments found")
	} else {
		discoveryLog.Info("Discovered target namespaces for service account creation",
			"count", len(namespaces),
			"namespaces", namespaces)
	}

	return namespaces, nil
}

func (nds NamespaceDiscoveryService) GetTargetNamespaces() []string {
	return []string{}
}
