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

package datasciencepipelines

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/module"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	operatorv1 "github.com/openshift/api/operator/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakePlatformObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	status        common.Status
	conditions    []common.Condition
	releaseStatus common.ComponentReleaseStatus
}

func (f *fakePlatformObject) DeepCopyObject() runtime.Object {
	out := *f
	return &out
}

func (f *fakePlatformObject) GetStatus() *common.Status {
	return &f.status
}

func (f *fakePlatformObject) GetConditions() []common.Condition {
	return f.conditions
}

func (f *fakePlatformObject) SetConditions(newConditions []common.Condition) {
	f.conditions = newConditions
}

func (f *fakePlatformObject) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &f.releaseStatus
}

func (f *fakePlatformObject) SetReleaseStatus(newStatus common.ComponentReleaseStatus) {
	f.releaseStatus = newStatus
}

func newFakeClient(
	t *testing.T,
) client.Client {
	t.Helper()

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(extv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(componentApi.AddToScheme(scheme)).To(Succeed())

	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func TestCheckPreConditions(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	tests := []struct {
		name                    string
		setupClient             func() client.Client
		instance                *componentApi.DataSciencePipelines
		expectedError           error
		expectedConditionStatus metav1.ConditionStatus
		expectedReason          string
		expectedMessage         string
	}{
		{
			name: "removed state fails when workflows CRD is missing",
			setupClient: func() client.Client {
				return newFakeClient(t)
			},
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
				Spec: componentApi.DataSciencePipelinesSpec{
					DataSciencePipelinesCommonSpec: componentApi.DataSciencePipelinesCommonSpec{
						ArgoWorkflowsControllers: &componentApi.ArgoWorkflowsControllersSpec{
							ManagementState: operatorv1.Removed,
						},
					},
				},
			},
			expectedError:           ErrArgoWorkflowCRDMissing,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedReason:          module.DataSciencePipelinesArgoWorkflowsCRDMissingReason,
			expectedMessage:         module.DataSciencePipelinesArgoWorkflowsCRDMissingMessage,
		},
		{
			name: "removed state passes when workflows CRD already exists",
			setupClient: func() client.Client {
				cli := newFakeClient(t)
				err := cli.Create(ctx, &extv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: ArgoWorkflowCRD},
				})
				g.Expect(err).NotTo(HaveOccurred())
				return cli
			},
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
				Spec: componentApi.DataSciencePipelinesSpec{
					DataSciencePipelinesCommonSpec: componentApi.DataSciencePipelinesCommonSpec{
						ArgoWorkflowsControllers: &componentApi.ArgoWorkflowsControllersSpec{
							ManagementState: operatorv1.Removed,
						},
					},
				},
			},
			expectedConditionStatus: metav1.ConditionTrue,
			expectedReason:          module.DataSciencePipelinesArgoWorkflowsNotManagedReason,
			expectedMessage:         module.DataSciencePipelinesArgoWorkflowsNotManagedMessage,
		},
		{
			name: "managed state passes when workflows CRD is missing",
			setupClient: func() client.Client {
				return newFakeClient(t)
			},
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
				Spec: componentApi.DataSciencePipelinesSpec{
					DataSciencePipelinesCommonSpec: componentApi.DataSciencePipelinesCommonSpec{
						ArgoWorkflowsControllers: &componentApi.ArgoWorkflowsControllersSpec{
							ManagementState: operatorv1.Managed,
						},
					},
				},
			},
			expectedConditionStatus: metav1.ConditionTrue,
		},
		{
			name: "managed state passes when workflows CRD is ODH-owned",
			setupClient: func() client.Client {
				cli := newFakeClient(t)
				err := cli.Create(ctx, &extv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: ArgoWorkflowCRD,
						Labels: map[string]string{
							appLabelPrefix + "/" + LegacyComponentName: "true",
						},
					},
				})
				g.Expect(err).NotTo(HaveOccurred())
				return cli
			},
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
				Spec: componentApi.DataSciencePipelinesSpec{
					DataSciencePipelinesCommonSpec: componentApi.DataSciencePipelinesCommonSpec{
						ArgoWorkflowsControllers: &componentApi.ArgoWorkflowsControllersSpec{
							ManagementState: operatorv1.Managed,
						},
					},
				},
			},
			expectedConditionStatus: metav1.ConditionTrue,
		},
		{
			name: "managed state fails when workflows CRD is foreign-owned",
			setupClient: func() client.Client {
				cli := newFakeClient(t)
				err := cli.Create(ctx, &extv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name:   ArgoWorkflowCRD,
						Labels: map[string]string{"some-other-label": "value"},
					},
				})
				g.Expect(err).NotTo(HaveOccurred())
				return cli
			},
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
				Spec: componentApi.DataSciencePipelinesSpec{
					DataSciencePipelinesCommonSpec: componentApi.DataSciencePipelinesCommonSpec{
						ArgoWorkflowsControllers: &componentApi.ArgoWorkflowsControllersSpec{
							ManagementState: operatorv1.Managed,
						},
					},
				},
			},
			expectedError:           ErrArgoWorkflowAPINotOwned,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedReason:          module.DataSciencePipelinesDoesntOwnArgoCRDReason,
			expectedMessage:         module.DataSciencePipelinesDoesntOwnArgoCRDMessage,
		},
		{
			name: "default managed behavior passes when option is omitted",
			setupClient: func() client.Client {
				return newFakeClient(t)
			},
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
			},
			expectedConditionStatus: metav1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := tt.setupClient()
			rr := fwtypes.ReconciliationRequest{
				Client:     cli,
				Instance:   tt.instance,
				Conditions: conditions.NewManager(tt.instance, string(fwapi.ConditionTypeReady)),
			}

			m := &Module{}
			err := m.checkPreConditions(ctx, &rr)
			if tt.expectedError != nil {
				g.Expect(err).To(Equal(tt.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			conditionChecks := []types.GomegaMatcher{
				HaveKeyWithValue("type", module.ConditionArgoWorkflowAvailable),
				HaveKeyWithValue("status", string(tt.expectedConditionStatus)),
			}
			if tt.expectedReason != "" {
				conditionChecks = append(conditionChecks, HaveKeyWithValue("reason", tt.expectedReason))
			}
			if tt.expectedMessage != "" {
				conditionChecks = append(conditionChecks, HaveKeyWithValue("message", tt.expectedMessage))
			}

			g.Expect(tt.instance).To(
				WithTransform(resources.ToUnstructured,
					WithTransform(k8s.Conditions(), SatisfyAll(
						ContainElement(SatisfyAll(conditionChecks...)),
					)),
				),
			)
		})
	}
}

func TestCheckPreConditionsWrongInstanceType(t *testing.T) {
	g := NewWithT(t)

	cli := newFakeClient(t)

	wrongInstance := &fakePlatformObject{
		ObjectMeta: metav1.ObjectMeta{Name: "wrong-type"},
	}
	rr := fwtypes.ReconciliationRequest{
		Client:     cli,
		Instance:   wrongInstance,
		Conditions: conditions.NewManager(wrongInstance, string(fwapi.ConditionTypeReady)),
	}

	m := &Module{}
	err := m.checkPreConditions(context.Background(), &rr)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("is not a DataSciencePipelines"))
}

func TestArgoWorkflowsControllersOptions(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name                string
		instance            common.PlatformObject
		expectedParamString string
		expectedError       bool
	}{
		{
			name:                "defaults to managed when option is omitted",
			instance:            newTestDataSciencePipelines(),
			expectedParamString: `ARGOWORKFLOWSCONTROLLERS={"managementState":"Managed"}`,
		},
		{
			name: "writes removed state when explicitly requested",
			instance: &componentApi.DataSciencePipelines{
				ObjectMeta: metav1.ObjectMeta{Name: componentApi.DataSciencePipelinesInstanceName},
				Spec: componentApi.DataSciencePipelinesSpec{
					DataSciencePipelinesCommonSpec: componentApi.DataSciencePipelinesCommonSpec{
						ArgoWorkflowsControllers: &componentApi.ArgoWorkflowsControllersSpec{
							ManagementState: operatorv1.Removed,
						},
					},
				},
			},
			expectedParamString: `ARGOWORKFLOWSCONTROLLERS={"managementState":"Removed"}`,
		},
		{
			name:          "fails for wrong instance type",
			instance:      &fakePlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "wrong-type"}},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTestParamsEnv(t)
			rr := fwtypes.ReconciliationRequest{
				Instance:          tt.instance,
				ManifestsBasePath: root,
			}

			cfg := testConfig(t)
			cfg.ManifestsPath = root
			m, buildErr := NewModule(cfg)
			g.Expect(buildErr).NotTo(HaveOccurred())

			err := m.argoWorkflowsControllersOptions(context.Background(), &rr)
			if tt.expectedError {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			content, readErr := m.renderFS.ReadFile(path.Join(componentName, "base", "params.env"))
			g.Expect(readErr).NotTo(HaveOccurred())
			g.Expect(string(content)).To(ContainSubstring(tt.expectedParamString))

			baseContent, baseReadErr := os.ReadFile(filepath.Join(root, componentName, "base", "params.env"))
			g.Expect(baseReadErr).NotTo(HaveOccurred())
			g.Expect(string(baseContent)).To(BeEmpty())
		})
	}
}
