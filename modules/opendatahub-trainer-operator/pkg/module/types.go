package module

import (
	helm "github.com/k8s-manifest-kit/renderer-helm/pkg"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	Name               = "trainer"
	OperatorConfigName = "odh-trainer-config"
	DescriptorPath     = "manifests/module.yaml"

	ConditionDependenciesAvailable = "DependenciesAvailable"
	PreConditionFailedReason       = "PreConditionFailed"

	JobSetOperatorNotInstalledMessage = "JobSet operator not installed, please install it first"
	JobSetCRDMissingMessage           = "" +
		"JobSet CRD does not exist, please inspect JobSetOperator CR status conditions " +
		"or JobSet controller Pod logs for more details"
	JobSetOperatorCRNotFoundMessage = "JobSetOperator CR with name 'cluster' not found, please create it first"

	VariantODH   = "odh"
	VariantRhoai = "rhoai"
)

type Descriptor struct {
	Variants map[string]Variant `yaml:"variants"`
}

type Variant struct {
	Manifests ManifestSet `yaml:"manifests"`
}

type ManifestSet struct {
	Kustomize []KustomizeItem `yaml:"kustomize,omitempty"`
	Templates []TemplateItem  `yaml:"templates,omitempty"`
	Helm      []HelmItem      `yaml:"helm,omitempty"`
}

type KustomizeItem struct {
	Path       string         `yaml:"path"`
	ContextDir string         `yaml:"contextDir,omitempty"`
	SourcePath string         `yaml:"sourcePath,omitempty"`
	Namespace  string         `yaml:"namespace,omitempty"`
	Params     []ParamsTarget `yaml:"params,omitempty"`
}

type ParamsTarget struct {
	// File is relative to the kustomize manifest root described by the parent item.
	File           string            `yaml:"file"`
	ReplaceFromEnv map[string]string `yaml:"replaceFromEnv,omitempty"`
	Values         map[string]string `yaml:"values,omitempty"`
}

type TemplateItem struct {
	Path        string            `yaml:"path"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type HelmItem struct {
	Repo                string `yaml:"repo,omitempty"`
	Chart               string `yaml:"chart"`
	ReleaseName         string `yaml:"releaseName"`
	ReleaseVersion      string `yaml:"releaseVersion,omitempty"`
	ProcessDependencies bool   `yaml:"processDependencies,omitempty"`
}

type ResolvedVariant struct {
	Name       string
	Kustomize  []ResolvedKustomizeItem
	Templates  []fwtypes.TemplateInfo
	HelmCharts []fwtypes.HelmChartInfo
}

type ResolvedParamsTarget struct {
	File           string
	ReplaceFromEnv map[string]string
	Values         map[string]string
}

type ResolvedKustomizeItem struct {
	ManifestInfo fwtypes.ManifestInfo
	Params       []ResolvedParamsTarget
}

func (h HelmItem) source() helm.Source {
	return helm.Source{
		Repo:                h.Repo,
		Chart:               h.Chart,
		ReleaseName:         h.ReleaseName,
		ReleaseVersion:      h.ReleaseVersion,
		ProcessDependencies: h.ProcessDependencies,
	}
}
