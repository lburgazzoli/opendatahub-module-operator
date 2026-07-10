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
	"io/fs"
	"os"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	serializeryaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyManifestFromFile reads a single YAML manifest from the local filesystem
// and applies it to the cluster. Existing resources are updated in place.
func ApplyManifestFromFile(
	ctx context.Context,
	cli client.Client,
	manifestPath string,
) (unstructured.Unstructured, error) {
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("reading manifest %s: %w", manifestPath, err)
	}

	obj, err := applyManifestBytes(ctx, cli, manifestBytes)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("applying manifest %s: %w", manifestPath, err)
	}

	return obj, nil
}

// ApplyManifestFromFS reads a single YAML manifest from the provided fs.FS and
// applies it to the cluster. Existing resources are updated in place.
func ApplyManifestFromFS(
	ctx context.Context,
	cli client.Client,
	fsys fs.FS,
	manifestPath string,
) (unstructured.Unstructured, error) {
	manifestBytes, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("reading manifest %s: %w", manifestPath, err)
	}

	obj, err := applyManifestBytes(ctx, cli, manifestBytes)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("applying manifest %s: %w", manifestPath, err)
	}

	return obj, nil
}

func applyManifestBytes(
	ctx context.Context,
	cli client.Client,
	manifestBytes []byte,
) (unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	decoder := serializeryaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	if _, _, err := decoder.Decode(manifestBytes, nil, obj); err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("decoding manifest: %w", err)
	}

	if obj.GroupVersionKind().Empty() {
		return unstructured.Unstructured{}, fmt.Errorf("validating manifest: manifest is missing apiVersion or kind")
	}

	if err := cli.Create(ctx, obj); err == nil {
		return *obj, nil
	} else if !k8serr.IsAlreadyExists(err) {
		return unstructured.Unstructured{}, fmt.Errorf("creating resource %s: %w", client.ObjectKeyFromObject(obj), err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	if err := cli.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("checking resource %s: %w", client.ObjectKeyFromObject(obj), err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	if err := cli.Update(ctx, obj); err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("updating resource %s: %w", client.ObjectKeyFromObject(obj), err)
	}

	return *obj, nil
}
