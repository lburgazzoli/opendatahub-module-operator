package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("trustyai"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/trustyai/overlays/odh/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"trustyaiServiceImage",
		"RELATED_IMAGE_ODH_TRUSTYAI_SERVICE_IMAGE",
	))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"evalHubImage",
		"RELATED_IMAGE_ODH_EVAL_HUB_IMAGE",
	))
	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantMCPGuardrails)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantMCPGuardrails))
	g.Expect(resolved.Kustomize).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/mcp-guardrails"))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal(
		"manifests/trustyai/overlays/mcp-guardrails/params.env",
	))
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
						ContextDir: "trustyai",
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

	g.Expect(os.Setenv("TRUSTYAI_SERVICE_IMAGE_ENV", "quay.io/example/trustyai-service:latest")).To(Succeed())
	g.Expect(os.Setenv("TRUSTYAI_MCP_IMAGE_ENV", "quay.io/example/nemo-guardrails:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("TRUSTYAI_SERVICE_IMAGE_ENV")
		_ = os.Unsetenv("TRUSTYAI_MCP_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/trustyai/overlays/odh")).To(Succeed())
	g.Expect(kfs.MkdirAll("manifests/trustyai/overlays/mcp-guardrails")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/trustyai/overlays/odh/params.env",
		[]byte("trustyaiServiceImage=\n"),
	)).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/trustyai/overlays/mcp-guardrails/params.env",
		[]byte("nemo-guardrails-image=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{
		{
			Params: []ResolvedParamsTarget{{
				File: "manifests/trustyai/overlays/odh/params.env",
				ReplaceFromEnv: map[string]string{
					"trustyaiServiceImage": "TRUSTYAI_SERVICE_IMAGE_ENV",
				},
			}},
		},
		{
			Params: []ResolvedParamsTarget{{
				File: "manifests/trustyai/overlays/mcp-guardrails/params.env",
				ReplaceFromEnv: map[string]string{
					"nemo-guardrails-image": "TRUSTYAI_MCP_IMAGE_ENV",
				},
			}},
		},
	}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())

	odhContent, err := kfs.ReadFile("manifests/trustyai/overlays/odh/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(odhContent)).To(ContainSubstring(
		"trustyaiServiceImage=quay.io/example/trustyai-service:latest\n",
	))

	mcpContent, err := kfs.ReadFile("manifests/trustyai/overlays/mcp-guardrails/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(mcpContent)).To(ContainSubstring(
		"nemo-guardrails-image=quay.io/example/nemo-guardrails:latest\n",
	))
}
