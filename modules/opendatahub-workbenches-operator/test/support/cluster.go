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
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	serializeryaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyYAMLs reads all YAML files from the given directory and applies them
// to the cluster. Existing resources are updated in place.
func ApplyYAMLs(
	ctx context.Context,
	cli client.Client,
	manifestDir string,
) error {
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		return fmt.Errorf("reading manifest directory %s: %w", manifestDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		manifestPath := filepath.Join(manifestDir, entry.Name())
		if err := ApplyYAML(ctx, cli, manifestPath); err != nil {
			return err
		}
	}

	return nil
}

// ApplyYAML reads one YAML manifest and applies it to the cluster. Existing
// resources are updated in place.
func ApplyYAML(
	ctx context.Context,
	cli client.Client,
	manifestPath string,
) error {
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest %s: %w", manifestPath, err)
	}

	obj := &unstructured.Unstructured{}
	decoder := serializeryaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	if _, _, err := decoder.Decode(manifestBytes, nil, obj); err != nil {
		return fmt.Errorf("decoding manifest %s: %w", manifestPath, err)
	}

	if obj.GroupVersionKind().Empty() {
		return fmt.Errorf("validating manifest %s: manifest is missing apiVersion or kind", manifestPath)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	if err := cli.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		if !k8serr.IsNotFound(err) {
			return fmt.Errorf("checking resource %s: %w", client.ObjectKeyFromObject(obj), err)
		}

		if err := cli.Create(ctx, obj); err != nil {
			return fmt.Errorf("creating resource %s: %w", client.ObjectKeyFromObject(obj), err)
		}

		return nil
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	if err := cli.Update(ctx, obj); err != nil {
		return fmt.Errorf("updating resource %s: %w", client.ObjectKeyFromObject(obj), err)
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
