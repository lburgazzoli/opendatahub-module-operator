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
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallCRDs reads all YAML files from the given directory and applies them
// as CustomResourceDefinitions to the cluster. Existing CRDs are updated
// to ensure our schema (including new status fields) takes effect.
func InstallCRDs(
	ctx context.Context,
	cli client.Client,
	crdDir string,
) error {
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		return fmt.Errorf("reading CRD directory %s: %w", crdDir, err)
	}

	crdScheme := runtime.NewScheme()
	utilruntime.Must(apiextensionsv1.AddToScheme(crdScheme))
	codecs := serializer.NewCodecFactory(crdScheme)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		crdBytes, err := os.ReadFile(filepath.Join(crdDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("reading CRD file %s: %w", entry.Name(), err)
		}

		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := runtime.DecodeInto(codecs.UniversalDeserializer(), crdBytes, crd); err != nil {
			return fmt.Errorf("decoding CRD from %s: %w", entry.Name(), err)
		}

		existing := &apiextensionsv1.CustomResourceDefinition{}
		if err := cli.Get(ctx, client.ObjectKeyFromObject(crd), existing); err != nil {
			if !k8serr.IsNotFound(err) {
				return fmt.Errorf("checking CRD %s: %w", crd.Name, err)
			}

			if err := cli.Create(ctx, crd); err != nil {
				return fmt.Errorf("creating CRD %s: %w", crd.Name, err)
			}

			continue
		}

		crd.ResourceVersion = existing.ResourceVersion
		if err := cli.Update(ctx, crd); err != nil {
			return fmt.Errorf("updating CRD %s: %w", crd.Name, err)
		}
	}

	return nil
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
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: plural[:len(plural)-1], // strip trailing 's' for singular
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
