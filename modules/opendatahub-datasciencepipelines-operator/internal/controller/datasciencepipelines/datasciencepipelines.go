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
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/module"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwerrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

const (
	ArgoWorkflowCRD     = "workflows.argoproj.io"
	componentName       = componentApi.DataSciencePipelinesComponentName
	LegacyComponentName = "data-science-pipelines-operator"

	platformVersionParamsKey          = "PLATFORMVERSION"
	fipsEnabledParamsKey              = "FIPSENABLED"
	argoWorkflowsControllersParamsKey = "ARGOWORKFLOWSCONTROLLERS"
)

var (
	ErrArgoWorkflowAPINotOwned = fwerrors.NewStopError(
		"Failed upgrade. DataSciencePipelines component found existing Argo Workflow CRD, which is not managed by ODH.",
	)
	ErrArgoWorkflowCRDMissing = fwerrors.NewStopError(
		"DataSciencePipelines component is configured not to manage Argo Workflow controllers, but workflows.argoproj.io CRD is missing.",
	)
)

type Module struct {
	cfg      *moduleconfig.Config
	release  fwapi.Release
	variant  modulemeta.ResolvedVariant
	renderFS filesys.FileSystem
}

func newKustomizeFS() (filesys.FileSystem, error) {
	baseKustomizeFS, err := kfs.NewFromIOFS(assets.Manifests, "")
	if err != nil {
		return nil, fmt.Errorf("creating base render filesystem: %w", err)
	}

	kustomizeFS, err := kfs.NewUnionFs(baseKustomizeFS)
	if err != nil {
		return nil, fmt.Errorf("creating render filesystem: %w", err)
	}

	return kustomizeFS, nil
}

func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	kustomizeFS, err := newKustomizeFS()
	if err != nil {
		return nil, err
	}

	variantName := modulemeta.VariantODH
	switch cfg.PlatformType {
	case moduleconfig.PlatformTypeSelfManagedRhoai, moduleconfig.PlatformTypeManagedRhoai:
		variantName = modulemeta.VariantRhoai
	}

	variant, err := modulemeta.LoadVariant(
		assets.Manifests,
		modulemeta.DescriptorPath,
		variantName,
	)
	if err != nil {
		return nil, fmt.Errorf("loading variant %q: %w", variantName, err)
	}

	return &Module{
		cfg:      cfg,
		variant:  variant,
		renderFS: kustomizeFS,
	}, nil
}

func (m *Module) Init(ctx context.Context, reader client.Reader) error {
	info, err := odhcluster.DetectClusterInfo(ctx, reader)
	if err != nil {
		return fmt.Errorf("detecting cluster info: %w", err)
	}

	if err := modulemeta.ApplyStaticParams(m.renderFS, m.variant.Kustomize); err != nil {
		return err
	}

	if err := modulemeta.ApplyRuntimeParams(m.renderFS, m.variant.Kustomize, map[string]string{
		platformVersionParamsKey: m.cfg.PlatformVersion.String(),
		fipsEnabledParamsKey:     strconv.FormatBool(info.FipsEnabled),
	}); err != nil {
		return fmt.Errorf("applying init params for variant %q: %w", m.variant.Name, err)
	}

	m.release = m.cfg.PlatformRelease()

	return nil
}
