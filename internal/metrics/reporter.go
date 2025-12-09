package metrics

import (
	"context"
	"os"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = logf.Log.WithName("metrics_reporter")

type MetricsReporter struct {
	c client.Client
}

func NewMetricsReporter(c client.Client) *MetricsReporter {
	return &MetricsReporter{
		c: c,
	}
}

func (m *MetricsReporter) Start(context.Context) error {
	opcNamespace, err := getInstallationNamespace(m.c)
	if err != nil {
		logger.V(0).Info("unable to get installation namespace", "error", err)
	} else {
		SetOpcNamespaceUid(string(opcNamespace.UID))
	}

	return nil
}

func getInstallationNamespace(k8sClient client.Client) (*v1.Namespace, error) {
	// Grab the name of the namespace that the controller is installed in
	installationNamespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return nil, err
	}

	opcNamespace := &v1.Namespace{}
	err = k8sClient.Get(context.TODO(), client.ObjectKey{Name: string(installationNamespace)}, opcNamespace)
	return opcNamespace, err
}
