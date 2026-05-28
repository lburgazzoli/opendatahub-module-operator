//go:build upgrade

/*
Copyright 2026.

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

package upgrade

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"k8s.io/client-go/rest"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	workbenchesmanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/manager"
	gvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
	workbenchesversion "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/version"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"
)

const (
	timeout   = 90 * time.Second
	interval  = 2 * time.Second
	retryWait = 20 * time.Second

	appliedUpgradeVersion = "0.1.0"
	desiredUpgradeVersion = "0.2.0"

	upgradeTriggerAnnotation = "opendatahub.io/upgrade-test-trigger"
	notebookSizeAnnotation   = "notebooks.opendatahub.io/last-size-selection"
	hwpNameAnnotation        = "opendatahub.io/hardware-profile-name"
	hwpNamespaceAnnotation   = "opendatahub.io/hardware-profile-namespace"

	moduleCRDName = "workbenches.components.platform.opendatahub.io"
)

var (
	ctx             context.Context
	cancel          context.CancelFunc
	kubeConfig      *rest.Config
	moduleCfg       *moduleconfig.Config
	directClient    client.Client
	k8sClient       client.Client
	k               *k8sm.Matcher
	directK         *k8sm.Matcher
	operatorCfgData map[string]string
	testScheme      = workbenchesmanager.NewScheme()
)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var err error

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	kubeConfig, err = config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		return 1
	}

	directClient, err = client.New(kubeConfig, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	operatorNamespace := support.OperatorNamespace()
	if err := support.EnsureNamespace(ctx, directClient, operatorNamespace); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create namespace: %v\n", err)
		return 1
	}

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
	}
	if err := directClient.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
		fmt.Fprintf(os.Stderr, "Expected CRD %s to be installed before running upgrade tests: %v\n", moduleCRDName, err)
		return 1
	}

	_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.Workbenches{})
	_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(operatorNamespace))
	_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(operatorNamespace))

	operatorCfgData = support.MustReadConfigMapData(
		support.MustProjectFile("config", "manager", "configmap.yaml"))

	workbenchesversion.Version = desiredUpgradeVersion

	moduleCfg = &moduleconfig.Config{
		PlatformType:          operatorCfgData[moduleconfig.KeyPlatformType],
		PlatformVersion:       desiredUpgradeVersion,
		MetricsAddr:           "0",
		HealthProbeAddr:       "0",
		LeaderElect:           false,
		ApplicationsNamespace: operatorNamespace,
		ManifestsPath:         support.MustProjectFile("config", "manifests"),
		WebhooksEnabled:       false,
	}

	directK = k8sm.New(directClient, testScheme)

	return m.Run()
}

func startManager(t *testing.T) {
	t.Helper()
	if k8sClient != nil {
		return
	}

	mgr, err := workbenchesmanager.New(
		ctx,
		kubeConfig,
		moduleCfg,
		func(opts *ctrl.Options) {
			opts.Cache.DefaultTransform = nil
			opts.Cache.ReaderFailOnMissingInformer = false
		},
	)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("failed to sync manager cache")
	}

	k8sClient = mgr.GetClient()
	k = k8sm.New(k8sClient, testScheme)
}

func TestWorkbenchesUpgradeContainerSizeMigration(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()

	module := &componentsv1alpha1.Workbenches{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.WorkbenchesInstanceName,
		},
		Spec: componentsv1alpha1.WorkbenchesSpec{
			WorkbenchesCommonSpec: componentsv1alpha1.WorkbenchesCommonSpec{
				WorkbenchNamespace: "opendatahub",
			},
		},
	}
	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
	}
	notebook := newNotebook(operatorNamespace, "upgrade-notebook")
	odhDashboardConfig := newOdhDashboardConfig(operatorNamespace)
	hardwareProfile := &infrav1.HardwareProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "containersize-small-notebooks",
			Namespace: operatorNamespace,
		},
	}

	deleteIfExists(t, notebook)
	deleteIfExists(t, odhDashboardConfig)
	deleteIfExists(t, hardwareProfile)
	deleteIfExists(t, module)
	waitForSingletonDeleted(t, module)

	t.Cleanup(func() {
		deleteIfExists(t, notebook)
		deleteIfExists(t, odhDashboardConfig)
		deleteIfExists(t, hardwareProfile)
		deleteIfExists(t, module)
	})

	g.Expect(directClient.Create(ctx, module)).To(Succeed())

	seededModule := &componentsv1alpha1.Workbenches{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.WorkbenchesInstanceName},
	}
	g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(seededModule), seededModule)).To(Succeed())
	seededModule.Status.Module.Version = componentsv1alpha1.SemVer(appliedUpgradeVersion)
	seededModule.Status.Module.Platform.Version = componentsv1alpha1.SemVer(appliedUpgradeVersion)
	g.Expect(directClient.Status().Update(ctx, seededModule)).To(Succeed())
	g.Expect(directClient.Create(ctx, odhDashboardConfig)).To(Succeed())
	g.Expect(directClient.Create(ctx, notebook)).To(Succeed())

	g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(seededModule), seededModule)).To(Succeed())
	g.Expect(seededModule.Status.Module.Version).To(Equal(componentsv1alpha1.SemVer(appliedUpgradeVersion)))
	g.Expect(seededModule.Status.Module.Platform.Version).To(Equal(componentsv1alpha1.SemVer(appliedUpgradeVersion)))
	g.Eventually(directK.Get(odhDashboardConfig)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "odh-dashboard-config"`),
		jq.Match(`.metadata.namespace == "%s"`, operatorNamespace),
	))
	g.Eventually(directK.Get(notebook)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "upgrade-notebook"`),
		jq.Match(`.metadata.namespace == "%s"`, operatorNamespace),
	))

	startManager(t)

	g.Eventually(k.Get(moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)

	g.Eventually(directK.Get(hardwareProfile)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "containersize-small-notebooks"`),
		jq.Match(`.metadata.namespace == "%s"`, operatorNamespace),
	))
	g.Eventually(directK.Get(notebook)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" == "Small"`, notebookSizeAnnotation),
		jq.Match(`.metadata.annotations."%s" == "containersize-small-notebooks"`, hwpNameAnnotation),
		jq.Match(`.metadata.annotations."%s" == "%s"`, hwpNamespaceAnnotation, operatorNamespace),
	))

	g.Eventually(directK.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.version == "%s"`, desiredUpgradeVersion),
	)
	g.Eventually(directK.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.platform.version == "%s"`, desiredUpgradeVersion),
	)
	g.Eventually(directK.Get(notebook)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" == "containersize-small-notebooks"`, hwpNameAnnotation),
		jq.Match(`.metadata.annotations."%s" == "%s"`, hwpNamespaceAnnotation, operatorNamespace),
	))
	g.Eventually(moduleHasEventReason(componentsv1alpha1.WorkbenchesInstanceName, "UpgradeStarted")).
		WithContext(ctx).
		WithTimeout(timeout).
		WithPolling(interval).
		Should(BeTrue())
}

func newNotebook(namespace string, name string) *unstructured.Unstructured {
	notebook := &unstructured.Unstructured{}
	notebook.SetGroupVersionKind(gvk.Notebook)
	notebook.SetName(name)
	notebook.SetNamespace(namespace)
	notebook.SetAnnotations(map[string]string{
		notebookSizeAnnotation: "Small",
	})
	notebook.Object["spec"] = map[string]any{
		"template": map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":            name,
						"image":           "quay.io/thoth-station/s2i-minimal-notebook:v0.2.2",
						"imagePullPolicy": "Always",
						"workingDir":      "/opt/app-root/src",
						"env": []any{
							map[string]any{"name": "JUPYTER_NOTEBOOK_PORT", "value": "8888"},
							map[string]any{"name": "NOTEBOOK_ARGS", "value": "--NotebookApp.token='' --NotebookApp.password=''"},
						},
					},
				},
			},
		},
	}
	return notebook
}

func newOdhDashboardConfig(namespace string) *unstructured.Unstructured {
	cfg := &unstructured.Unstructured{}
	cfg.SetGroupVersionKind(gvk.OdhDashboardConfig)
	cfg.SetName("odh-dashboard-config")
	cfg.SetNamespace(namespace)
	cfg.Object["spec"] = map[string]any{
		"notebookSizes": []any{
			map[string]any{
				"name": "Small",
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    "1",
						"memory": "8Gi",
					},
					"limits": map[string]any{
						"cpu":    "2",
						"memory": "8Gi",
					},
				},
			},
		},
		"notebookController": map[string]any{
			"enabled": true,
		},
	}
	return cfg
}

func triggerUpgradeReconcile(t *testing.T, moduleName string) {
	t.Helper()

	g := NewWithT(t)
	module := &componentsv1alpha1.Workbenches{
		ObjectMeta: metav1.ObjectMeta{Name: moduleName},
	}

	g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(module), module)).To(Succeed())
	module.Status.Module.Version = componentsv1alpha1.SemVer(appliedUpgradeVersion)
	module.Status.Module.Platform.Version = componentsv1alpha1.SemVer(appliedUpgradeVersion)
	g.Expect(directClient.Status().Update(ctx, module)).To(Succeed())
	g.Eventually(directK.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version == "%s"`, appliedUpgradeVersion),
		jq.Match(`.status.module.platform.version == "%s"`, appliedUpgradeVersion),
	))

	g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(module), module)).To(Succeed())
	annotations := module.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[upgradeTriggerAnnotation] = strconv.FormatInt(time.Now().UnixNano(), 10)
	module.SetAnnotations(annotations)
	g.Expect(directClient.Update(ctx, module)).To(Succeed())
}

func triggerUpgradeReconcileUntilHardwareProfile(
	t *testing.T,
	moduleName string,
	hardwareProfile *infrav1.HardwareProfile,
) {
	t.Helper()

	var lastErr error
	for range 3 {
		triggerUpgradeReconcile(t, moduleName)
		lastErr = waitForObjectDirect(hardwareProfile, retryWait)
		if lastErr == nil {
			return
		}
	}

	t.Fatalf(
		"hardware profile %s/%s was not created after repeated upgrade reconciles: %v",
		hardwareProfile.GetNamespace(),
		hardwareProfile.GetName(),
		lastErr,
	)
}

func moduleHasEventReason(moduleName string, reason string) func() bool {
	return func() bool {
		events := &corev1.EventList{}
		if err := directClient.List(ctx, events, client.InNamespace(support.OperatorNamespace())); err != nil {
			return false
		}
		for i := range events.Items {
			event := &events.Items[i]
			if event.Reason != reason {
				continue
			}
			if event.InvolvedObject.Kind != componentsv1alpha1.WorkbenchesKind {
				continue
			}
			if event.InvolvedObject.Name != moduleName {
				continue
			}
			return true
		}
		return false
	}
}

func waitForObjectDirect(obj client.Object, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		ctx,
		interval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			err := directClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
			if err == nil {
				return true, nil
			}
			if k8serr.IsNotFound(err) {
				return false, nil
			}

			return false, err
		},
	)
}

func deleteIfExists(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	err := directClient.Delete(ctx, obj)
	g.Expect(err).To(SatisfyAny(
		BeNil(),
		MatchError(k8serr.IsNotFound, "IsNotFound"),
		MatchError(meta.IsNoMatchError, "IsNoMatchError"),
	))
}

func waitForDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(directK.Gone(obj)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())
}

func waitForSingletonDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	waitForDeleted(t, obj)
	obj.SetResourceVersion("")
	obj.SetUID("")
}

func eventuallyDeploymentReady(t *testing.T, deploy *appsv1.Deployment) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(k.Get(deploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}
