package module

import (
	"fmt"
	"io/fs"
	"maps"
	"path"
	"strings"

	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type Resolver struct {
	manifestFS fs.FS
	desc       Descriptor
}

func LoadDescriptor(manifestFS fs.FS, descriptorPath string) (Descriptor, error) {
	if manifestFS == nil {
		return Descriptor{}, fmt.Errorf("manifest filesystem is nil")
	}

	data, err := fs.ReadFile(manifestFS, descriptorPath)
	if err != nil {
		return Descriptor{}, fmt.Errorf("reading descriptor %s: %w", descriptorPath, err)
	}

	desc := Descriptor{}
	if err := yaml.Unmarshal(data, &desc); err != nil {
		return Descriptor{}, fmt.Errorf("parsing descriptor %s: %w", descriptorPath, err)
	}

	if len(desc.Variants) == 0 {
		return Descriptor{}, fmt.Errorf("descriptor %s has no variants", descriptorPath)
	}

	return desc, nil
}

func NewResolver(manifestFS fs.FS, desc Descriptor) (*Resolver, error) {
	if manifestFS == nil {
		return nil, fmt.Errorf("manifest filesystem is nil")
	}
	if len(desc.Variants) == 0 {
		return nil, fmt.Errorf("descriptor has no variants")
	}

	return &Resolver{
		manifestFS: manifestFS,
		desc:       desc,
	}, nil
}

func LoadVariant(
	manifestFS fs.FS,
	descriptorPath string,
	variant string,
) (ResolvedVariant, error) {
	desc, err := LoadDescriptor(manifestFS, descriptorPath)
	if err != nil {
		return ResolvedVariant{}, err
	}

	resolver, err := NewResolver(manifestFS, desc)
	if err != nil {
		return ResolvedVariant{}, err
	}

	return resolver.Resolve(variant)
}

func (r *Resolver) Resolve(variant string) (ResolvedVariant, error) {
	v, ok := r.desc.Variants[variant]
	if !ok {
		return ResolvedVariant{}, fmt.Errorf("variant %q not found", variant)
	}

	resolved := ResolvedVariant{
		Name:       variant,
		Kustomize:  make([]ResolvedKustomizeItem, 0, len(v.Manifests.Kustomize)),
		Templates:  make([]fwtypes.TemplateInfo, 0, len(v.Manifests.Templates)),
		HelmCharts: make([]fwtypes.HelmChartInfo, 0, len(v.Manifests.Helm)),
	}

	for _, item := range v.Manifests.Kustomize {
		kustomizeItem := ResolvedKustomizeItem{
			ManifestInfo: fwtypes.ManifestInfo{
				Path:       item.Path,
				ContextDir: item.ContextDir,
				SourcePath: item.SourcePath,
				Namespace:  item.Namespace,
			},
			Params: make([]ResolvedParamsTarget, 0, len(item.Params)),
		}
		for _, target := range item.Params {
			file, err := resolveParamsFile(item, target.File)
			if err != nil {
				return ResolvedVariant{}, err
			}

			kustomizeItem.Params = append(kustomizeItem.Params, ResolvedParamsTarget{
				File:           file,
				ReplaceFromEnv: maps.Clone(target.ReplaceFromEnv),
				Values:         maps.Clone(target.Values),
			})
		}
		resolved.Kustomize = append(resolved.Kustomize, kustomizeItem)
	}

	for _, item := range v.Manifests.Templates {
		resolved.Templates = append(resolved.Templates, fwtypes.TemplateInfo{
			FS:          r.manifestFS,
			Path:        item.Path,
			Labels:      maps.Clone(item.Labels),
			Annotations: maps.Clone(item.Annotations),
		})
	}

	for _, item := range v.Manifests.Helm {
		resolved.HelmCharts = append(resolved.HelmCharts, fwtypes.HelmChartInfo{
			Source: item.source(),
		})
	}

	return resolved, nil
}

func resolveParamsFile(item KustomizeItem, file string) (string, error) {
	clean := path.Clean(file)
	if clean == "." {
		return "", fmt.Errorf("params file for %q is empty", item.SourcePath)
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("params file %q must be relative to the manifest root", file)
	}

	return path.Join(item.Path, item.ContextDir, item.SourcePath, clean), nil
}

func ApplyStaticParams(kustomizeFS filesys.FileSystem, items []ResolvedKustomizeItem) error {
	return applyParams(kustomizeFS, items, func(target ResolvedParamsTarget) []kparams.Mapper {
		mappers := make([]kparams.Mapper, 0, 2)
		if len(target.ReplaceFromEnv) != 0 {
			mappers = append(mappers, kparams.Replacement(kparams.FromEnv(target.ReplaceFromEnv)))
		}
		if len(target.Values) != 0 {
			mappers = append(mappers, kparams.Values(target.Values))
		}
		return mappers
	})
}

// ApplyRuntimeParams broadcasts the provided runtime values to every resolved
// params target. An empty values map is a no-op.
func ApplyRuntimeParams(
	kustomizeFS filesys.FileSystem,
	items []ResolvedKustomizeItem,
	values map[string]string,
) error {
	return applyParams(kustomizeFS, items, func(_ ResolvedParamsTarget) []kparams.Mapper {
		if len(values) == 0 {
			return nil
		}

		return []kparams.Mapper{kparams.Values(values)}
	})
}

func applyParams(
	kustomizeFS filesys.FileSystem,
	items []ResolvedKustomizeItem,
	mapperFn func(ResolvedParamsTarget) []kparams.Mapper,
) error {
	for _, item := range items {
		for _, target := range item.Params {
			mappers := mapperFn(target)
			if len(mappers) == 0 {
				continue
			}

			if err := kparams.Apply(kustomizeFS, target.File, mappers...); err != nil {
				return fmt.Errorf("applying params on %s: %w", target.File, err)
			}
		}
	}

	return nil
}
