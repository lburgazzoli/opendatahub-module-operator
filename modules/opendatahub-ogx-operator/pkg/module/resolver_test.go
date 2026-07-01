package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/assets"
)

func TestLoadDescriptorAndResolve(t *testing.T) {
	g := NewWithT(t)

	desc, err := LoadDescriptor(assets.Manifests, DescriptorPath)
	g.Expect(err).NotTo(HaveOccurred())

	resolver, err := NewResolver(assets.Manifests, desc)
	g.Expect(err).NotTo(HaveOccurred())

	resolved, err := resolver.Resolve(VariantODH)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(resolved.Name).To(Equal(VariantODH))
	g.Expect(resolved.Kustomize).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].ManifestInfo.Path).To(Equal("manifests"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("ogx"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/ogx/overlays/odh/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(Equal(map[string]string{
		"RELATED_IMAGE_ODH_OGX_OPERATOR": "RELATED_IMAGE_ODH_OGX_K8S_OPERATOR_IMAGE",
		"RELATED_IMAGE_RH_DISTRIBUTION":  "RELATED_IMAGE_ODH_OGX_CORE_IMAGE",
	}))
	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantRhoai)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantRhoai))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/rhoai"))
}

func TestResolveRejectsUnknownVariant(t *testing.T) {
	g := NewWithT(t)

	desc, err := LoadDescriptor(assets.Manifests, DescriptorPath)
	g.Expect(err).NotTo(HaveOccurred())

	resolver, err := NewResolver(assets.Manifests, desc)
	g.Expect(err).NotTo(HaveOccurred())

	_, err = resolver.Resolve("does-not-exist")
	g.Expect(err).To(MatchError(ContainSubstring(`variant "does-not-exist" not found`)))
}

func TestResolveRejectsUnsafeParamsFile(t *testing.T) {
	g := NewWithT(t)

	resolver, err := NewResolver(assets.Manifests, Descriptor{
		Variants: map[string]Variant{
			VariantODH: {
				Manifests: ManifestSet{
					Kustomize: []KustomizeItem{{
						Path:       "manifests",
						ContextDir: "ogx",
						SourcePath: "overlays/odh",
						Params: []ParamsTarget{{
							File: "../params.env",
						}},
					}},
				},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	_, err = resolver.Resolve(VariantODH)
	g.Expect(err).To(MatchError(ContainSubstring(`must be relative to the manifest root`)))
}

func TestApplyStaticParams(t *testing.T) {
	g := NewWithT(t)

	g.Expect(os.Setenv("OGX_OPERATOR_IMAGE_ENV", "quay.io/example/ogx-operator:latest")).To(Succeed())
	g.Expect(os.Setenv("OGX_CORE_IMAGE_ENV", "quay.io/example/ogx-core:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("OGX_OPERATOR_IMAGE_ENV")
		_ = os.Unsetenv("OGX_CORE_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/ogx/overlays/odh")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/ogx/overlays/odh/params.env",
		[]byte("RELATED_IMAGE_ODH_OGX_OPERATOR=\nRELATED_IMAGE_RH_DISTRIBUTION=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		Params: []ResolvedParamsTarget{{
			File: "manifests/ogx/overlays/odh/params.env",
			ReplaceFromEnv: map[string]string{
				"RELATED_IMAGE_ODH_OGX_OPERATOR": "OGX_OPERATOR_IMAGE_ENV",
				"RELATED_IMAGE_RH_DISTRIBUTION":  "OGX_CORE_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())

	content, err := kfs.ReadFile("manifests/ogx/overlays/odh/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring(
		"RELATED_IMAGE_ODH_OGX_OPERATOR=quay.io/example/ogx-operator:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring(
		"RELATED_IMAGE_RH_DISTRIBUTION=quay.io/example/ogx-core:latest\n",
	))
}
