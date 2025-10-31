package helpers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega" // nolint:revive,staticcheck
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// EnvTestConfig holds configuration for setting up an envtest environment
type EnvTestConfig struct {
	// CRDDirectoryPaths are paths to directories containing CRDs
	CRDDirectoryPaths []string
	// ErrorIfCRDPathMissing determines if an error should be thrown if CRD path is missing
	ErrorIfCRDPathMissing bool
	// WebhookPaths are paths to directories containing webhook configurations (optional, for webhook tests)
	WebhookPaths []string
	// Scheme is the runtime scheme to use
	Scheme *runtime.Scheme
}

// EnvTestResult holds the result of setting up an envtest environment
type EnvTestResult struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	K8sClient client.Client
	Cfg       *rest.Config
	TestEnv   *envtest.Environment
}

// SetupEnvTest sets up a basic envtest environment for controller/rules tests
func SetupEnvTest(config EnvTestConfig) *EnvTestResult {
	ctx, cancel := context.WithCancel(context.TODO())

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     config.CRDDirectoryPaths,
		ErrorIfCRDPathMissing: config.ErrorIfCRDPathMissing,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if GetFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = GetFirstFoundEnvTestBinaryDir()
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err := client.New(cfg, client.Options{Scheme: config.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	return &EnvTestResult{
		Ctx:       ctx,
		Cancel:    cancel,
		K8sClient: k8sClient,
		Cfg:       cfg,
		TestEnv:   testEnv,
	}
}

// TearDownEnvTest tears down the envtest environment
func TearDownEnvTest(result *EnvTestResult) {
	result.Cancel()
	err := result.TestEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
}

// WebhookTestConfig holds configuration specific to webhook tests
type WebhookTestConfig struct {
	EnvTestConfig
	// SetupWebhookFunc is a function that sets up webhooks on the manager
	SetupWebhookFunc func(mgr ctrl.Manager) error
}

// SetupEnvTestWithWebhook sets up an envtest environment with webhook support
func SetupEnvTestWithWebhook(config WebhookTestConfig) *EnvTestResult {
	ctx, cancel := context.WithCancel(context.TODO())

	webhookInstallOptions := envtest.WebhookInstallOptions{
		Paths: config.WebhookPaths,
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     config.CRDDirectoryPaths,
		ErrorIfCRDPathMissing: config.ErrorIfCRDPathMissing,
		WebhookInstallOptions: webhookInstallOptions,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if GetFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = GetFirstFoundEnvTestBinaryDir()
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err := client.New(cfg, client.Options{Scheme: config.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Start webhook server using Manager
	webhookInstallOptionsPtr := &testEnv.WebhookInstallOptions
	logf.Log.Info("Setting up webhook server")
	logf.Log.Info(fmt.Sprintf("WebhookInstallOptions: %#v", webhookInstallOptionsPtr))
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: config.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptionsPtr.LocalServingHost,
			Port:    webhookInstallOptionsPtr.LocalServingPort,
			CertDir: webhookInstallOptionsPtr.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).NotTo(HaveOccurred())

	// Setup webhooks if provided
	if config.SetupWebhookFunc != nil {
		err = config.SetupWebhookFunc(mgr)
		Expect(err).NotTo(HaveOccurred())
	}

	// Start manager in background
	go func() {
		err = mgr.Start(ctx)
		if err != nil {
			logf.Log.Error(err, "Failed to start manager")
		}
	}()

	// Wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptionsPtr.LocalServingHost, webhookInstallOptionsPtr.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		return conn.Close()
	}).Should(Succeed())

	return &EnvTestResult{
		Ctx:       ctx,
		Cancel:    cancel,
		K8sClient: k8sClient,
		Cfg:       cfg,
		TestEnv:   testEnv,
	}
}

// DefaultCRDPaths returns the standard CRD paths for different test locations
func DefaultCRDPaths(levelsUp int) []string {
	pathPrefix := ""
	for i := 0; i < levelsUp; i++ {
		pathPrefix = filepath.Join(pathPrefix, "..")
	}
	return []string{filepath.Join(pathPrefix, "config", "crd", "bases")}
}

// DefaultWebhookPaths returns the standard webhook paths for different test locations
func DefaultWebhookPaths(levelsUp int) []string {
	pathPrefix := ""
	for i := 0; i < levelsUp; i++ {
		pathPrefix = filepath.Join(pathPrefix, "..")
	}
	return []string{filepath.Join(pathPrefix, "config", "webhook")}
}
