/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	lanv1beta1 "github.com/hujun-open/k8slan/api/v1beta1"
	ncv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/common"
	webhookv1beta1 "kubenetlab.net/knl/internal/webhook/v1beta1"
	kvv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

const (
	testControllerNS = "knl-system"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = knlv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	// testEnv = &envtest.Environment{
	// 	CRDDirectoryPaths: []string{
	// 		filepath.Join("..", "..", "config", "crd", "bases"),
	// 		"../../testdata/3rdpartyCR/",
	// 	},
	// 	ErrorIfCRDPathMissing: true,
	// 	//enable webhook
	// 	WebhookInstallOptions: envtest.WebhookInstallOptions{
	// 		Paths: []string{filepath.Join("..", "..", "config", "webhook")},
	// 	},
	// }
	testEnv = &envtest.Environment{
		UseExistingCluster: common.ReturnPointerVal(true),
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kvv1.AddToScheme(scheme))
	utilruntime.Must(ncv1.AddToScheme(scheme))
	utilruntime.Must(knlv1beta1.AddToScheme(scheme))
	utilruntime.Must(lanv1beta1.AddToScheme(scheme))
	utilruntime.Must(cdiv1.AddToScheme(scheme))
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
	//create controller

	//create some env, so that manager code believe it lives in ns knl-system
	os.Setenv("WATCH_NAMESPACE", testControllerNS)
	os.Setenv("KNL_HTTP_PORT", "80")
	testNS := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testControllerNS,
			Namespace: testControllerNS,
		},
	}
	By("Creating the Namespace to perform the tests")
	err = k8sClient.Get(ctx, types.NamespacedName{Name: testControllerNS}, &v1.Namespace{})
	if err != nil && errors.IsNotFound(err) {
		err = k8sClient.Create(ctx, testNS)
		Expect(err).NotTo(HaveOccurred())
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// Metrics:                metricsServerOptions,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    testEnv.WebhookInstallOptions.LocalServingHost,
			Port:    testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
		}),
		// HealthProbeBindAddress: probeAddr,
		// LeaderElection:         enableLeaderElection,
		// LeaderElectionID:       "edd8af43.kubenetlab.net",
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				testControllerNS: {},
			},
			// This tells the manager: "For Secrets, only watch this specific namespace"
			// All other resources (like your CRDs) will remain cluster-scoped.
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Namespaces: map[string]cache.Config{
						knlv1beta1.MYNAMESPACE: {},
					},
				},
			},
		},
	})
	if err := (&LabReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	if err := (&KNLConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	if err := webhookv1beta1.SetupLabWebhookWithManager(mgr); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	if err := webhookv1beta1.SetupKNLConfigWebhookWithManager(mgr); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
