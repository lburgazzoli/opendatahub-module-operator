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
	"path/filepath"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	serializeryaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var installCRDs = []string{
	"config/crd/bases/infrastructure.opendatahub.io_databaseclaims.yaml",
	"config/crd/bases/infrastructure.opendatahub.io_databaseproviders.yaml",
	"config/crd/bases/infrastructure.opendatahub.io_schemaclaims.yaml",
	"config/crd/bases/services.platform.opendatahub.io_databaseservices.yaml",
}

// InstallCRD installs this module's CRDs into the provided cluster client and
// waits for each one to become established before moving to the next.
func InstallCRD(ctx context.Context, cli client.Client) error {
	for _, manifest := range installCRDs {
		manifestPath, err := ModulePath(manifest)
		if err != nil {
			return fmt.Errorf("resolving CRD manifest path %q: %w", manifest, err)
		}

		obj, err := applyManifestFromFile(ctx, cli, manifestPath)
		if err != nil {
			return fmt.Errorf("installing CRD manifest %q: %w", manifest, err)
		}

		if err := wait.PollUntilContextTimeout(
			ctx,
			500*time.Millisecond,
			60*time.Second,
			true,
			func(ctx context.Context) (bool, error) {
				return isCRDEstablished(ctx, cli, obj.GetName())
			},
		); err != nil {
			return fmt.Errorf("waiting for CRD %q to become established: %w", obj.GetName(), err)
		}
	}

	return nil
}

// applyManifestFromFile reads a single YAML manifest from the local filesystem
// and applies it to the cluster. Existing resources are updated in place.
func applyManifestFromFile(
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

// applyManifestFromFS reads a single YAML manifest from the provided fs.FS and
// applies it to the cluster. Existing resources are updated in place.
func applyManifestFromFS(
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

func isCRDEstablished(
	ctx context.Context,
	cli client.Client,
	name string,
) (bool, error) {
	present, crd, err := lookupCRD(ctx, cli, name)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}

	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}

func HasCRD(
	ctx context.Context,
	cli client.Client,
	name string,
) (bool, error) {
	present, _, err := lookupCRD(ctx, cli, name)
	return present, err
}

func lookupCRD(
	ctx context.Context,
	cli client.Client,
	name string,
) (bool, *apiextensionsv1.CustomResourceDefinition, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: name}, crd); err != nil {
		if k8serr.IsNotFound(err) {
			return false, nil, nil
		}

		return false, nil, fmt.Errorf("getting CRD %q: %w", name, err)
	}

	return true, crd, nil
}

func ModulePath(parts ...string) (string, error) {
	root, err := ModuleRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(append([]string{root}, parts...)...), nil
}

func ModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent directory")
		}

		dir = parent
	}
}
