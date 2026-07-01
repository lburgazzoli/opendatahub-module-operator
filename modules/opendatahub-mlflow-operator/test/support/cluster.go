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

	modulegvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/resources/gvk"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gatewayConfigCRDName  = "gatewayconfigs.services.platform.opendatahub.io"
	gatewayConfigResource = "gatewayconfigs"
	gatewayConfigName     = "default-gateway"
	testStubLabelKey      = "opendatahub.io/test-stub"
)

type MLflowPreconditions struct {
	ManageGatewayConfigCR bool
}

// EnsureStubCRD creates a minimal CRD stub so that cluster.HasCRD returns true.
// If the CRD already exists, the returned error wraps the underlying
// AlreadyExists error so callers can detect ownership without a separate Get.
func EnsureStubCRD(
	ctx context.Context,
	cli client.Client,
	crdName string,
	group string,
	version string,
	kind string,
	plural string,
) error {
	preserveUnknownFields := true

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
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
						Type:                   "object",
						XPreserveUnknownFields: &preserveUnknownFields,
					},
				},
			}},
		},
	}

	if err := cli.Create(ctx, crd); err != nil {
		return fmt.Errorf("creating stub CRD %s: %w", crdName, err)
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
	if err := EnsureStubCRD(ctx, cli, crdName, group, version, kind, plural); err != nil {
		if k8serr.IsAlreadyExists(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func newStubGatewayConfigCRD() *apiextensionsv1.CustomResourceDefinition {
	preserveUnknownFields := true

	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: gatewayConfigCRDName,
			Labels: map[string]string{
				testStubLabelKey: "true",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: modulegvk.GatewayConfig.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   gatewayConfigResource,
				Singular: "gatewayconfig",
				Kind:     modulegvk.GatewayConfig.Kind,
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    modulegvk.GatewayConfig.Version,
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"status": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"domain": {Type: "string"},
								},
							},
						},
						XPreserveUnknownFields: &preserveUnknownFields,
					},
				},
				Subresources: &apiextensionsv1.CustomResourceSubresources{
					Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
				},
			}},
		},
	}
}

func ownsStubGatewayConfigCRD(crd *apiextensionsv1.CustomResourceDefinition) bool {
	if crd.GetLabels()[testStubLabelKey] == "true" {
		return true
	}

	if len(crd.Spec.Versions) != 1 {
		return false
	}

	version := crd.Spec.Versions[0]
	return version.Subresources == nil &&
		version.Schema != nil &&
		version.Schema.OpenAPIV3Schema != nil &&
		version.Schema.OpenAPIV3Schema.XPreserveUnknownFields != nil &&
		*version.Schema.OpenAPIV3Schema.XPreserveUnknownFields
}

func EnsureStubGatewayConfigCRDIfMissing(
	ctx context.Context,
	cli client.Client,
) (bool, error) {
	current := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: gatewayConfigCRDName},
	}
	if err := cli.Get(ctx, client.ObjectKeyFromObject(current), current); err == nil {
		if !ownsStubGatewayConfigCRD(current) {
			return false, nil
		}

		desired := newStubGatewayConfigCRD()
		desired.ResourceVersion = current.ResourceVersion
		if err := cli.Update(ctx, desired); err != nil {
			return false, fmt.Errorf("updating stub CRD %s: %w", gatewayConfigCRDName, err)
		}
		return true, nil
	} else if !k8serr.IsNotFound(err) {
		return false, err
	}

	if err := cli.Create(ctx, newStubGatewayConfigCRD()); err != nil {
		return false, fmt.Errorf("creating stub CRD %s: %w", gatewayConfigCRDName, err)
	}

	return true, nil
}

func NewStubGatewayConfig() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(modulegvk.GatewayConfig)
	u.SetName(gatewayConfigName)
	u.SetLabels(map[string]string{
		testStubLabelKey: "true",
	})
	return u
}

func ownsStubGatewayConfig(obj *unstructured.Unstructured) bool {
	return obj.GetLabels()[testStubLabelKey] == "true"
}

// EnsureStubGatewayConfig creates or updates the GatewayConfig singleton used by
// MLflow reconcile-time gateway URL rendering.
func EnsureStubGatewayConfig(ctx context.Context, cli client.Client) error {
	deadline := time.Now().Add(30 * time.Second)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for GatewayConfig %s to report status.domain", gatewayConfigName)
		}

		current := NewStubGatewayConfig()
		err := cli.Get(ctx, client.ObjectKeyFromObject(current), current)
		switch {
		case err == nil:
		case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
			toCreate := NewStubGatewayConfig()
			if err := cli.Create(ctx, toCreate); err != nil &&
				!k8serr.IsAlreadyExists(err) &&
				!apimeta.IsNoMatchError(err) {
				return fmt.Errorf("creating stub GatewayConfig %s: %w", gatewayConfigName, err)
			}
		default:
			return fmt.Errorf("getting stub GatewayConfig %s: %w", gatewayConfigName, err)
		}

		current = NewStubGatewayConfig()
		if err := cli.Get(ctx, client.ObjectKeyFromObject(current), current); err != nil {
			if (k8serr.IsNotFound(err) || apimeta.IsNoMatchError(err)) && time.Now().Before(deadline) {
				time.Sleep(2 * time.Second)
				continue
			}
			return fmt.Errorf("waiting for stub GatewayConfig %s: %w", gatewayConfigName, err)
		}

		domain, found, err := unstructured.NestedString(current.Object, "status", "domain")
		if err != nil {
			return fmt.Errorf("reading GatewayConfig.status.domain: %w", err)
		}
		if found && domain == GatewayConfigDomain() {
			return nil
		}

		base := current.DeepCopy()
		if err := unstructured.SetNestedField(current.Object, GatewayConfigDomain(), "status", "domain"); err != nil {
			return fmt.Errorf("setting GatewayConfig.status.domain: %w", err)
		}

		if err := cli.Status().Patch(ctx, current, client.MergeFrom(base)); err != nil {
			if apimeta.IsNoMatchError(err) || k8serr.IsNotFound(err) {
				if time.Now().Before(deadline) {
					time.Sleep(2 * time.Second)
					continue
				}
				return fmt.Errorf("waiting for GatewayConfig status subresource: %w", err)
			}
			if err := cli.Update(ctx, current); err != nil {
				return fmt.Errorf("updating stub GatewayConfig %s: %w", gatewayConfigName, err)
			}
		}
	}
}

func EnsureStubGatewayConfigIfMissing(
	ctx context.Context,
	cli client.Client,
	ownsGatewayConfigCRD bool,
) (bool, error) {
	obj := NewStubGatewayConfig()
	if err := cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err == nil {
		if ownsGatewayConfigCRD || ownsStubGatewayConfig(obj) {
			return false, EnsureStubGatewayConfig(ctx, cli)
		}
		return false, nil
	} else if !k8serr.IsNotFound(err) && !apimeta.IsNoMatchError(err) {
		return false, err
	}

	if err := EnsureStubGatewayConfig(ctx, cli); err != nil {
		return false, err
	}

	return true, nil
}

func EnsureMLflowPreconditionsIfMissing(
	ctx context.Context,
	cli client.Client,
) (MLflowPreconditions, error) {
	ownsGatewayConfigCRD, err := EnsureStubGatewayConfigCRDIfMissing(ctx, cli)
	if err != nil {
		return MLflowPreconditions{}, err
	}

	manageGatewayConfigCR, err := EnsureStubGatewayConfigIfMissing(ctx, cli, ownsGatewayConfigCRD)
	if err != nil {
		return MLflowPreconditions{}, err
	}

	return MLflowPreconditions{
		ManageGatewayConfigCR: manageGatewayConfigCR,
	}, nil
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
