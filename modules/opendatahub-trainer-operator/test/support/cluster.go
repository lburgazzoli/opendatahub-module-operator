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

package support

import (
	"context"
	"fmt"
	"time"

	modulegvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/resources/gvk"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stubResourceLabelKey   = "testing.opendatahub.io/stub-resource"
	stubResourceLabelValue = "trainer"
	jobSetOperatorCRDName  = "jobsetoperators.operator.openshift.io"
	jobSetCRDName          = "jobsets.jobset.x-k8s.io"
)

type TrainerPreconditions struct {
	ManageJobSetOperatorCRD bool
	ManageJobSetOperatorCR  bool
	ManageJobSetCRD         bool
}

// EnsureStubCRD creates a minimal CRD stub so that cluster.HasCRD returns true.
// Used in integration tests to satisfy precondition checks without installing the full operand.
func EnsureStubCRD(
	ctx context.Context,
	cli client.Client,
	crdName string,
	group string,
	version string,
	kind string,
	plural string,
) error {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: crdName,
			Labels: map[string]string{
				stubResourceLabelKey: stubResourceLabelValue,
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: plural[:len(plural)-1],
				Kind:     kind,
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    version,
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
					},
				},
			}},
		},
	}

	if err := cli.Create(ctx, crd); err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("creating stub CRD %s: %w", crdName, err)
	}

	return nil
}

// NewStubJobSetOperatorCR returns a minimal unstructured JobSetOperator CR for use in integration tests.
func NewStubJobSetOperatorCR() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(modulegvk.JobSetOperatorV1)
	u.SetName("cluster")
	u.SetLabels(map[string]string{
		stubResourceLabelKey: stubResourceLabelValue,
	})
	return u
}

// EnsureStubJobSetOperatorCR creates the stub JobSetOperator CR if it does not already exist.
// Retries for up to 30 s to tolerate API server discovery lag after CRD creation.
func EnsureStubJobSetOperatorCR(ctx context.Context, cli client.Client) error {
	cr := NewStubJobSetOperatorCR()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := cli.Create(ctx, cr); err == nil || k8serr.IsAlreadyExists(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for JobSetOperator CRD to become discoverable")
		}
		time.Sleep(2 * time.Second)
	}
}

// EnsureNamespace creates a namespace if it does not already exist.
func EnsureNamespace(
	ctx context.Context,
	cli client.Client,
	name string,
) error {
	ns := &corev1.Namespace{}
	ns.Name = name

	if err := cli.Create(ctx, ns); err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", name, err)
	}

	return nil
}

func EnsureStubCRDIfMissing(
	ctx context.Context,
	cli client.Client,
	crdName string,
	group string,
	version string,
	kind string,
	plural string,
) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
	}
	if err := cli.Get(ctx, client.ObjectKeyFromObject(crd), crd); err == nil {
		return false, nil
	} else if !k8serr.IsNotFound(err) {
		return false, err
	}

	if err := EnsureStubCRD(ctx, cli, crdName, group, version, kind, plural); err != nil {
		return false, err
	}

	return true, nil
}

func EnsureStubJobSetOperatorCRIfMissing(
	ctx context.Context,
	cli client.Client,
) (bool, error) {
	cr := NewStubJobSetOperatorCR()
	if err := cli.Get(ctx, client.ObjectKeyFromObject(cr), cr); err == nil {
		return false, nil
	} else if !k8serr.IsNotFound(err) && !apimeta.IsNoMatchError(err) {
		return false, err
	}

	if err := EnsureStubJobSetOperatorCR(ctx, cli); err != nil {
		return false, err
	}

	return true, nil
}

func EnsureTrainerPreconditions(
	ctx context.Context,
	cli client.Client,
) error {
	if err := EnsureStubCRD(
		ctx,
		cli,
		jobSetOperatorCRDName,
		"operator.openshift.io",
		"v1",
		"JobSetOperator",
		"jobsetoperators",
	); err != nil {
		return err
	}
	if err := EnsureStubJobSetOperatorCR(ctx, cli); err != nil {
		return err
	}
	if err := EnsureStubCRD(
		ctx,
		cli,
		jobSetCRDName,
		"jobset.x-k8s.io",
		"v1alpha2",
		"JobSet",
		"jobsets",
	); err != nil {
		return err
	}

	return nil
}

func EnsureTrainerPreconditionsIfMissing(
	ctx context.Context,
	cli client.Client,
) (TrainerPreconditions, error) {
	state := TrainerPreconditions{}

	var err error
	if state.ManageJobSetOperatorCRD, err = EnsureStubCRDIfMissing(
		ctx,
		cli,
		jobSetOperatorCRDName,
		"operator.openshift.io",
		"v1",
		"JobSetOperator",
		"jobsetoperators",
	); err != nil {
		return TrainerPreconditions{}, err
	}
	if state.ManageJobSetOperatorCR, err = EnsureStubJobSetOperatorCRIfMissing(ctx, cli); err != nil {
		return TrainerPreconditions{}, err
	}
	if state.ManageJobSetCRD, err = EnsureStubCRDIfMissing(
		ctx,
		cli,
		jobSetCRDName,
		"jobset.x-k8s.io",
		"v1alpha2",
		"JobSet",
		"jobsets",
	); err != nil {
		return TrainerPreconditions{}, err
	}

	return state, nil
}
