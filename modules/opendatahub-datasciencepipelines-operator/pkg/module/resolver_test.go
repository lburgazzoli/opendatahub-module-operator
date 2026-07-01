package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/assets"
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
	g.Expect(resolved.Kustomize).To(HaveLen(2))
	g.Expect(resolved.Kustomize[0].ManifestInfo.Path).To(Equal("manifests"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("datasciencepipelines"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].SkipRender).To(BeFalse())
	g.Expect(resolved.Kustomize[0].Params).To(BeEmpty())

	g.Expect(resolved.Kustomize[1].ManifestInfo.SourcePath).To(Equal("base"))
	g.Expect(resolved.Kustomize[1].SkipRender).To(BeTrue())
	g.Expect(resolved.Kustomize[1].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[1].Params[0].File).To(Equal("manifests/datasciencepipelines/base/params.env"))
	g.Expect(resolved.Kustomize[1].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"IMAGES_DSPO",
		"RELATED_IMAGE_ODH_DATA_SCIENCE_PIPELINES_OPERATOR_CONTROLLER_IMAGE",
	))
	g.Expect(resolved.Kustomize[1].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"IMAGES_ARGO_WORKFLOWCONTROLLER",
		"RELATED_IMAGE_ODH_DATA_SCIENCE_PIPELINES_ARGO_WORKFLOWCONTROLLER_IMAGE",
	))

	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantRhoai)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantRhoai))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/rhoai"))
	g.Expect(resolved.Kustomize[1].ManifestInfo.SourcePath).To(Equal("base"))
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
						ContextDir: "datasciencepipelines",
						SourcePath: "base",
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

	g.Expect(os.Setenv("DSPO_IMAGE_ENV", "quay.io/example/dspo:latest")).To(Succeed())
	g.Expect(os.Setenv("ARGO_CONTROLLER_IMAGE_ENV", "quay.io/example/argo-controller:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("DSPO_IMAGE_ENV")
		_ = os.Unsetenv("ARGO_CONTROLLER_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/datasciencepipelines/base")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/datasciencepipelines/base/params.env",
		[]byte("IMAGES_DSPO=\nIMAGES_ARGO_WORKFLOWCONTROLLER=\nPLATFORMVERSION=\nFIPSENABLED=\nARGOWORKFLOWSCONTROLLERS=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		SkipRender: true,
		Params: []ResolvedParamsTarget{{
			File: "manifests/datasciencepipelines/base/params.env",
			ReplaceFromEnv: map[string]string{
				"IMAGES_DSPO":                    "DSPO_IMAGE_ENV",
				"IMAGES_ARGO_WORKFLOWCONTROLLER": "ARGO_CONTROLLER_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())
	g.Expect(ApplyRuntimeParams(kfs, items, map[string]string{
		"PLATFORMVERSION":          "1.0.0",
		"FIPSENABLED":              "true",
		"ARGOWORKFLOWSCONTROLLERS": "{\"managementState\":\"Managed\"}",
	})).To(Succeed())

	content, err := kfs.ReadFile("manifests/datasciencepipelines/base/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring("IMAGES_DSPO=quay.io/example/dspo:latest\n"))
	g.Expect(string(content)).To(ContainSubstring(
		"IMAGES_ARGO_WORKFLOWCONTROLLER=quay.io/example/argo-controller:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring("PLATFORMVERSION=1.0.0\n"))
	g.Expect(string(content)).To(ContainSubstring("FIPSENABLED=true\n"))
	g.Expect(string(content)).To(ContainSubstring(
		"ARGOWORKFLOWSCONTROLLERS={\"managementState\":\"Managed\"}\n",
	))
}
