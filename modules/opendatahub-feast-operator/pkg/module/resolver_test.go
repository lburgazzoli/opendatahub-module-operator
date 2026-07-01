package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("feastoperator"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/feastoperator/overlays/odh/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(Equal(map[string]string{
		"RELATED_IMAGE_FEAST_OPERATOR": "RELATED_IMAGE_ODH_FEAST_OPERATOR_IMAGE",
		"RELATED_IMAGE_FEATURE_SERVER": "RELATED_IMAGE_ODH_FEATURE_SERVER_IMAGE",
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
						ContextDir: "feastoperator",
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

func TestApplyStaticAndRuntimeParams(t *testing.T) {
	g := NewWithT(t)

	g.Expect(os.Setenv("FEAST_OPERATOR_IMAGE_ENV", "quay.io/example/feast-operator:latest")).To(Succeed())
	g.Expect(os.Setenv("FEATURE_SERVER_IMAGE_ENV", "quay.io/example/feature-server:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("FEAST_OPERATOR_IMAGE_ENV")
		_ = os.Unsetenv("FEATURE_SERVER_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/feastoperator/overlays/odh")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/feastoperator/overlays/odh/params.env",
		[]byte("RELATED_IMAGE_FEAST_OPERATOR=\nRELATED_IMAGE_FEATURE_SERVER=\nOIDC_ISSUER_URL=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		Params: []ResolvedParamsTarget{{
			File: "manifests/feastoperator/overlays/odh/params.env",
			ReplaceFromEnv: map[string]string{
				"RELATED_IMAGE_FEAST_OPERATOR": "FEAST_OPERATOR_IMAGE_ENV",
				"RELATED_IMAGE_FEATURE_SERVER": "FEATURE_SERVER_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())
	g.Expect(ApplyRuntimeParams(kfs, items, map[string]string{
		"OIDC_ISSUER_URL": "https://issuer.example.com",
	})).To(Succeed())

	content, err := kfs.ReadFile("manifests/feastoperator/overlays/odh/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring("RELATED_IMAGE_FEAST_OPERATOR=quay.io/example/feast-operator:latest\n"))
	g.Expect(string(content)).To(ContainSubstring("RELATED_IMAGE_FEATURE_SERVER=quay.io/example/feature-server:latest\n"))
	g.Expect(string(content)).To(ContainSubstring("OIDC_ISSUER_URL=https://issuer.example.com\n"))
}
