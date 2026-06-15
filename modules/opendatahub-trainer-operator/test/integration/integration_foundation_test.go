package integration

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/test/support"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
)

const (
	jobSetOperatorCRDName = "jobsetoperators.operator.openshift.io"
	jobSetCRDName         = "jobsets.jobset.x-k8s.io"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should become ready", ft.testBecomesReady)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should report not ready when JobSet operator CRD is missing", ft.testJobSetOperatorCRDMissing)
	t.Run("should report not ready when JobSet operator CR is missing", ft.testJobSetOperatorCRMissing)
	t.Run("should report not ready when JobSet CRD is missing", ft.testJobSetCRDMissing)
}

func (ft *foundationTests) cleanupModuleWorkload(t *testing.T) {
	t.Helper()

	trainer := &componentsv1alpha1.Trainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.TrainerInstanceName,
		},
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadDeploy.Name,
			Namespace: workloadDeploy.Namespace,
		},
	}

	_ = ft.Client.Delete(t.Context(), trainer)
	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, trainer)).Should(BeTrue())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, workloadDeploy)).Should(BeTrue())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, serviceAccount)).Should(BeTrue())
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.Trainer {
	t.Helper()

	g := NewWithT(t)
	trainer := &componentsv1alpha1.Trainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.TrainerInstanceName,
		},
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}

	_ = ft.Client.Delete(t.Context(), trainer)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, trainer)).Should(BeTrue())

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), trainer)
	})

	g.Expect(ft.Client.Create(t.Context(), trainer)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, trainer)).Should(
		WithTransform(k8sm.Conditions(), SatisfyAll(
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "Ready"),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "ProvisioningSucceeded"),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
		)),
	)

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	return trainer
}

func (ft *foundationTests) expectDependenciesUnavailable(
	t *testing.T,
	obj *componentsv1alpha1.Trainer,
	expectedMessage string,
) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, obj)).Should(
		WithTransform(k8sm.Conditions(), SatisfyAll(
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", modulemeta.ConditionDependenciesAvailable),
				HaveKeyWithValue("status", metav1.ConditionFalse),
				HaveKeyWithValue("reason", modulemeta.PreConditionFailedReason),
				HaveKeyWithValue("message", expectedMessage),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", fwapi.ConditionTypeReady),
				HaveKeyWithValue("status", metav1.ConditionFalse),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", common.ConditionTypeProvisioningSucceeded),
				HaveKeyWithValue("status", metav1.ConditionFalse),
			)),
		)),
	)
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.TrainerCRDName},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, moduleCRD)).Should(Succeed())
}

func (ft *foundationTests) testJobSetOperatorCRDMissing(t *testing.T) {
	g := NewWithT(t)
	obj := &componentsv1alpha1.Trainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.TrainerInstanceName,
		},
	}
	ft.cleanupModuleWorkload(t)

	jobSetOperatorCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetOperatorCRDName},
	}
	g.Expect(ft.Client.Delete(t.Context(), jobSetOperatorCRD)).To(Succeed())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, jobSetOperatorCRD)).Should(BeTrue())
	t.Cleanup(func() {
		_, _ = support.EnsureStubCRDIfMissing(
			t.Context(),
			ft.Client,
			jobSetOperatorCRDName,
			"operator.openshift.io",
			"v1",
			"JobSetOperator",
			"jobsetoperators",
		)
		_, _ = support.EnsureStubJobSetOperatorCRIfMissing(t.Context(), ft.Client)
		ft.cleanupModuleWorkload(t)
	})

	g.Expect(ft.Client.Create(t.Context(), obj)).To(Succeed())
	ft.expectDependenciesUnavailable(t, obj, modulemeta.JobSetOperatorNotInstalledMessage)
}

func (ft *foundationTests) testJobSetOperatorCRMissing(t *testing.T) {
	g := NewWithT(t)
	obj := &componentsv1alpha1.Trainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.TrainerInstanceName,
		},
	}
	ft.cleanupModuleWorkload(t)

	jobSetOperatorCR := support.NewStubJobSetOperatorCR()
	g.Expect(ft.Client.Delete(t.Context(), jobSetOperatorCR)).To(Succeed())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, jobSetOperatorCR)).Should(BeTrue())
	t.Cleanup(func() {
		_, _ = support.EnsureStubJobSetOperatorCRIfMissing(t.Context(), ft.Client)
		ft.cleanupModuleWorkload(t)
	})

	g.Expect(ft.Client.Create(t.Context(), obj)).To(Succeed())
	ft.expectDependenciesUnavailable(t, obj, modulemeta.JobSetOperatorCRNotFoundMessage)
}

func (ft *foundationTests) testJobSetCRDMissing(t *testing.T) {
	g := NewWithT(t)
	obj := &componentsv1alpha1.Trainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.TrainerInstanceName,
		},
	}
	ft.cleanupModuleWorkload(t)

	jobSetCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetCRDName},
	}
	g.Expect(ft.Client.Delete(t.Context(), jobSetCRD)).To(Succeed())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, jobSetCRD)).Should(BeTrue())
	t.Cleanup(func() {
		_, _ = support.EnsureStubCRDIfMissing(
			t.Context(),
			ft.Client,
			jobSetCRDName,
			"jobset.x-k8s.io",
			"v1alpha2",
			"JobSet",
			"jobsets",
		)
		ft.cleanupModuleWorkload(t)
	})

	g.Expect(ft.Client.Create(t.Context(), obj)).To(Succeed())
	ft.expectDependenciesUnavailable(t, obj, modulemeta.JobSetCRDMissingMessage)
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	ft.ensureReadyModule(t)
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	g := NewWithT(t)

	trainer := ft.ensureReadyModule(t)
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, trainer)).Should(And(
		jq.Matchf(`.status.release.version == "%s"`, cfg.Release().Version.String()),
		jq.Matchf(`.status.release.name == "%s"`, cfg.PlatformName),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	trainer := ft.ensureReadyModule(t)
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.TrainerComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, trainer.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(trainer.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformType, cfg.PlatformName),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.Release().Version.String()),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	owner := ft.ensureReadyModule(t)
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	owner.TypeMeta = metav1.TypeMeta{
		APIVersion: componentsv1alpha1.GroupVersion.String(),
		Kind:       componentsv1alpha1.TrainerKind,
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
}
