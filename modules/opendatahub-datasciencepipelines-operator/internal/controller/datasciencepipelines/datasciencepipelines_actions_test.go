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
	"path/filepath"
	"testing"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	upstreamcomponents "github.com/opendatahub-io/opendatahub-operator/v2/api/components/v1alpha1"
	operatorv1 "github.com/openshift/api/operator/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/conditions"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/fakeclient"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/utils/test/matchers/jq"
)

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
				cli, err := fakeclient.New()
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
			expectedError:           ErrArgoWorkflowCRDMissing,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedReason:          status.DataSciencePipelinesArgoWorkflowsCRDMissingReason,
			expectedMessage:         status.DataSciencePipelinesArgoWorkflowsCRDMissingMessage,
		},
		{
			name: "removed state passes when workflows CRD already exists",
			setupClient: func() client.Client {
				cli, err := fakeclient.New()
				g.Expect(err).NotTo(HaveOccurred())
				err = cli.Create(ctx, &extv1.CustomResourceDefinition{
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
			expectedReason:          status.DataSciencePipelinesArgoWorkflowsNotManagedReason,
			expectedMessage:         status.DataSciencePipelinesArgoWorkflowsNotManagedMessage,
		},
		{
			name: "managed state passes when workflows CRD is missing",
			setupClient: func() client.Client {
				cli, err := fakeclient.New()
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
			name: "managed state passes when workflows CRD is ODH-owned",
			setupClient: func() client.Client {
				cli, err := fakeclient.New()
				g.Expect(err).NotTo(HaveOccurred())
				err = cli.Create(ctx, &extv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: ArgoWorkflowCRD,
						Labels: map[string]string{
							labels.ODH.Component(LegacyComponentName): "true",
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
				cli, err := fakeclient.New()
				g.Expect(err).NotTo(HaveOccurred())
				err = cli.Create(ctx, &extv1.CustomResourceDefinition{
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
			expectedReason:          status.DataSciencePipelinesDoesntOwnArgoCRDReason,
			expectedMessage:         status.DataSciencePipelinesDoesntOwnArgoCRDMessage,
		},
		{
			name: "default managed behavior passes when option is omitted",
			setupClient: func() client.Client {
				cli, err := fakeclient.New()
				g.Expect(err).NotTo(HaveOccurred())
				return cli
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
			rr := odhtypes.ReconciliationRequest{
				Client:     cli,
				Instance:   tt.instance,
				Conditions: conditions.NewManager(tt.instance, status.ConditionTypeReady),
			}

			err := checkPreConditions(ctx, &rr)
			if tt.expectedError != nil {
				g.Expect(err).To(Equal(tt.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			g.Expect(tt.instance).To(
				WithTransform(resources.ToUnstructured, jq.Match(
					`.status.conditions[] | select(.type == "%s") | .status == "%s"`,
					status.ConditionArgoWorkflowAvailable,
					tt.expectedConditionStatus,
				)),
			)

			if tt.expectedReason != "" {
				g.Expect(tt.instance).To(
					WithTransform(resources.ToUnstructured, jq.Match(
						`.status.conditions[] | select(.type == "%s") | .reason == "%s"`,
						status.ConditionArgoWorkflowAvailable,
						tt.expectedReason,
					)),
				)
			}

			if tt.expectedMessage != "" {
				g.Expect(tt.instance).To(
					WithTransform(resources.ToUnstructured, jq.Match(
						`.status.conditions[] | select(.type == "%s") | .message == "%s"`,
						status.ConditionArgoWorkflowAvailable,
						tt.expectedMessage,
					)),
				)
			}
		})
	}
}

func TestCheckPreConditionsWrongInstanceType(t *testing.T) {
	g := NewWithT(t)

	cli, err := fakeclient.New()
	g.Expect(err).NotTo(HaveOccurred())

	wrongInstance := &upstreamcomponents.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "wrong-type"},
	}
	rr := odhtypes.ReconciliationRequest{
		Client:     cli,
		Instance:   wrongInstance,
		Conditions: conditions.NewManager(wrongInstance, status.ConditionTypeReady),
	}

	err = checkPreConditions(context.Background(), &rr)
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
			instance:      &upstreamcomponents.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "wrong-type"}},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTestParamsEnv(t)
			rr := odhtypes.ReconciliationRequest{
				Instance:          tt.instance,
				ManifestsBasePath: root,
			}

			err := argoWorkflowsControllersOptions(context.Background(), &rr)
			if tt.expectedError {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			content, readErr := os.ReadFile(filepath.Join(root, componentName, "base", "params.env"))
			g.Expect(readErr).NotTo(HaveOccurred())
			g.Expect(string(content)).To(ContainSubstring(tt.expectedParamString))
		})
	}
}
