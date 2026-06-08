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

package platform

import (
	"errors"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/conditions"
	odhTypes "github.com/opendatahub-io/operator-actions-framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnabledModulesAtRunlevelFiltersBySpec(t *testing.T) {
	g := NewWithT(t)

	actions := testActions()
	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: []string{"alpha", "beta"},
		},
	}

	modules := actions.enabledModulesAtRunlevel(p, 2)

	g.Expect(modules).To(HaveLen(1))
	g.Expect(modules[0].EffectiveName()).To(Equal("beta"))
}

func TestRunlevelCompleteIgnoresDisabledModulesAtSameRunlevel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 2)
	beta := newPlatformOperator("beta", "2.0.0")
	rr := testRR(t, scheme, p, beta)

	complete, err := actions.runlevelComplete(ctx, rr, p, 2)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(complete).To(BeTrue())
}

func TestRunlevelCompleteBlocksOnMissingEnabledModule(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 2)
	rr := testRR(t, scheme, p)

	complete, err := actions.runlevelComplete(ctx, rr, p, 2)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(complete).To(BeFalse())
}

func TestAdvanceRunlevelSkipsRunlevelsWithoutDesiredModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "delta"}, 1)
	alpha := newPlatformOperator("alpha", "2.0.0")
	rr := testRR(t, scheme, p, alpha)

	err := actions.advanceRunlevel(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(p.Status.Runlevel).To(Equal(3))
}

func TestAdvanceRunlevelStaysWhenEnabledModuleVersionIsOutdated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 1)
	alpha := newPlatformOperator("alpha", "wrong-version")
	rr := testRR(t, scheme, p, alpha)

	err := actions.advanceRunlevel(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(p.Status.Runlevel).To(Equal(1))
}

func TestPruneModulesDeletesDisabledPlatformOperators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha"}, 1)
	alpha := newPlatformOperator("alpha", "2.0.0")
	beta := newPlatformOperator("beta", "2.0.0")
	rr := testRR(t, scheme, p, alpha, beta)

	err := actions.pruneModules(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "alpha"}, &configApi.PlatformOperator{})).To(Succeed())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "beta"}, &configApi.PlatformOperator{})).
		To(Satisfy(k8serr.IsNotFound))
}

func TestPruneModulesKeepsDesiredPlatformOperators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 1)
	alpha := newPlatformOperator("alpha", "2.0.0")
	beta := newPlatformOperator("beta", "2.0.0")
	rr := testRR(t, scheme, p, alpha, beta)

	err := actions.pruneModules(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "alpha"}, &configApi.PlatformOperator{})).To(Succeed())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "beta"}, &configApi.PlatformOperator{})).To(Succeed())
}

func TestCheckAdminAcks(t *testing.T) {
	t.Run("allows modules without admin acks", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		scheme := testScheme(t)
		actions := testActions()
		p := newPlatform([]string{"alpha"}, 1)
		rr := testRR(t, scheme, p)

		err := actions.checkAdminAcks(ctx, rr)

		g.Expect(err).NotTo(HaveOccurred())
		expectNoModulesReadyCondition(t, p)
	})

	t.Run("missing configmap pauses gated modules", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		scheme := testScheme(t)
		actions := testActions()
		actions.registry.ModuleByName("alpha").AdminAcks = []module.AdminAck{{
			Name:        "adminAcks.alpha",
			Description: "Acknowledge alpha rollout",
		}}
		p := newPlatform([]string{"alpha"}, 1)
		rr := testRR(t, scheme, p)

		err := actions.checkAdminAcks(ctx, rr)

		var pauseErr actionerrors.PauseError
		g.Expect(errors.As(err, &pauseErr)).To(BeTrue())
		g.Expect(pauseErr.Delay()).To(Equal(30 * time.Second))
		g.Expect(err.Error()).To(ContainSubstring("adminAcks.alpha"))
		g.Expect(err.Error()).To(ContainSubstring("modules: alpha"))
		g.Expect(err.Error()).To(ContainSubstring("description: Acknowledge alpha rollout"))
		expectModulesReadyCondition(t, p, metav1.ConditionFalse, "AdminAcksRequired", "Acknowledge alpha rollout")
	})

	t.Run("false ack pauses gated modules", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		scheme := testScheme(t)
		actions := testActions()
		actions.registry.ModuleByName("alpha").AdminAcks = []module.AdminAck{{
			Name:        "adminAcks.alpha",
			Description: "Acknowledge alpha rollout",
		}}
		p := newPlatform([]string{"alpha"}, 1)
		cm := newAdminAcksConfigMap(actions.cfg.Namespace(), map[string]string{"adminAcks.alpha": "false"})
		rr := testRR(t, scheme, p, cm)

		err := actions.checkAdminAcks(ctx, rr)

		var pauseErr actionerrors.PauseError
		g.Expect(errors.As(err, &pauseErr)).To(BeTrue())
		g.Expect(err.Error()).To(ContainSubstring(`value: "false"`))
		g.Expect(err.Error()).To(ContainSubstring("description: Acknowledge alpha rollout"))
		expectModulesReadyCondition(t, p, metav1.ConditionFalse, "AdminAcksRequired", `value: "false"`)
	})

	t.Run("true ack allows gated modules", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		scheme := testScheme(t)
		actions := testActions()
		actions.registry.ModuleByName("alpha").AdminAcks = []module.AdminAck{{
			Name:        "adminAcks.alpha",
			Description: "Acknowledge alpha rollout",
		}}
		p := newPlatform([]string{"alpha"}, 1)
		cm := newAdminAcksConfigMap(actions.cfg.Namespace(), map[string]string{"adminAcks.alpha": "true"})
		rr := testRR(t, scheme, p, cm)

		err := actions.checkAdminAcks(ctx, rr)

		g.Expect(err).NotTo(HaveOccurred())
		expectNoModulesReadyCondition(t, p)
	})

	t.Run("emits a warning event per unsatisfied ack", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		scheme := testScheme(t)
		recorder := events.NewFakeRecorder(10)
		actions := testActions()
		actions.recorder = recorder
		actions.registry.ModuleByName("alpha").AdminAcks = []module.AdminAck{{
			Name:        "adminAcks.alpha",
			Description: "Acknowledge alpha rollout",
		}}
		p := newPlatform([]string{"alpha"}, 1)
		rr := testRR(t, scheme, p)

		err := actions.checkAdminAcks(ctx, rr)

		var pauseErr actionerrors.PauseError
		g.Expect(errors.As(err, &pauseErr)).To(BeTrue())
		g.Eventually(recorder.Events).Should(Receive(SatisfyAll(
			ContainSubstring("AdminAckRequired"),
			ContainSubstring("Acknowledge alpha rollout"),
		)))
	})

	t.Run("deduplicates merged requirements across enabled modules", func(t *testing.T) {
		g := NewWithT(t)
		actions := testActions()
		actions.registry.ModuleByName("alpha").AdminAcks = []module.AdminAck{
			{Name: "adminAcks.shared", Description: "Shared admin gate"},
			{Name: "adminAcks.alpha", Description: "Alpha gate"},
		}
		actions.registry.ModuleByName("beta").AdminAcks = []module.AdminAck{
			{Name: "adminAcks.shared", Description: "Shared admin gate"},
		}
		p := newPlatform([]string{"alpha", "beta"}, 1)

		required := actions.requiredAdminAcks(p)

		g.Expect(required).To(HaveLen(2))
		g.Expect(required).To(HaveKeyWithValue("adminAcks.alpha", Equal(adminAckRequirement{
			Name:        "adminAcks.alpha",
			Description: "Alpha gate",
			Modules:     []string{"alpha"},
		})))
		g.Expect(required).To(HaveKeyWithValue("adminAcks.shared", Equal(adminAckRequirement{
			Name:        "adminAcks.shared",
			Description: "Shared admin gate",
			Modules:     []string{"alpha", "beta"},
		})))
	})
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	err = configApi.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("add config api to scheme: %v", err)
	}

	return scheme
}

func testActions() *PlatformReconciler {
	return &PlatformReconciler{
		registry: testRegistry(),
		cfg: &config.Config{
			Distribution: config.DistributionConfig{
				Name:    "TestPlatform",
				Version: "2.0.0",
			},
		},
	}
}

func expectModulesReadyCondition(
	t *testing.T,
	p *configApi.Platform,
	status metav1.ConditionStatus,
	reason string,
	messageSubstring string,
) {
	t.Helper()
	g := NewWithT(t)

	var condition *common.Condition
	for i := range p.GetConditions() {
		if p.GetConditions()[i].Type == ConditionModulesReady {
			condition = &p.GetConditions()[i]
			break
		}
	}

	g.Expect(condition).NotTo(BeNil())
	g.Expect(condition.Status).To(Equal(status))
	g.Expect(condition.Reason).To(Equal(reason))
	if messageSubstring != "" {
		g.Expect(condition.Message).To(ContainSubstring(messageSubstring))
	}
}

func expectNoModulesReadyCondition(t *testing.T, p *configApi.Platform) {
	t.Helper()
	g := NewWithT(t)

	for i := range p.GetConditions() {
		g.Expect(p.GetConditions()[i].Type).NotTo(Equal(ConditionModulesReady))
	}
}

func testRegistry() *module.ModuleRegistry {
	r := module.NewModuleRegistry("opendatahub", "/charts")
	r.Register(&module.Module{
		Name:      "alpha",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "ns-alpha",
		Runlevel:  1,
	})
	r.Register(&module.Module{
		Name:      "beta",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Beta"},
		Namespace: "ns-beta",
		Runlevel:  2,
	})
	r.Register(&module.Module{
		Name:      "gamma",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Gamma"},
		Namespace: "ns-gamma",
		Runlevel:  2,
	})
	r.Register(&module.Module{
		Name:      "delta",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Delta"},
		Namespace: "ns-delta",
		Runlevel:  3,
	})
	r.ComputeRunlevels()

	return r
}

func testRR(
	t *testing.T,
	scheme *runtime.Scheme,
	platform *configApi.Platform,
	objs ...client.Object,
) *odhTypes.ReconciliationRequest {
	t.Helper()

	allObjs := make([]client.Object, 0, len(objs)+1)
	allObjs = append(allObjs, objs...)
	allObjs = append(allObjs, platform)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(allObjs...).
		Build()

	return &odhTypes.ReconciliationRequest{
		Client:     c,
		Conditions: conditions.NewManager(platform, ConditionModulesReady),
		Instance:   platform,
	}
}

func newPlatform(modules []string, runlevel int) *configApi.Platform {
	return &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: modules,
		},
		Status: configApi.PlatformStatus{
			Runlevel: runlevel,
			Distribution: configApi.DistributionInfo{
				Name:    "TestPlatform",
				Version: "1.0.0",
			},
		},
	}
}

func newPlatformOperator(name string, version string) *configApi.PlatformOperator {
	return &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: configApi.PlatformOperatorStatus{
			Distribution: configApi.DistributionInfo{
				Name:    "TestPlatform",
				Version: version,
			},
		},
	}
}

func newAdminAcksConfigMap(namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AdminAcksConfigMapName,
			Namespace: namespace,
		},
		Data: data,
	}
}
