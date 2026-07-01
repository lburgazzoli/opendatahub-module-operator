package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("modelregistry"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.Namespace).To(BeEmpty())
	g.Expect(resolved.Kustomize[1].ManifestInfo.SourcePath).To(Equal("overlays/odh/extras"))
	g.Expect(resolved.Kustomize[1].Params).To(BeEmpty())

	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))

	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/modelregistry/overlays/odh/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(Equal(map[string]string{
		"IMAGES_MODELREGISTRY_OPERATOR": "RELATED_IMAGE_ODH_MODEL_REGISTRY_OPERATOR_IMAGE",
		"IMAGES_REST_SERVICE":           "RELATED_IMAGE_ODH_MODEL_REGISTRY_IMAGE",
		"IMAGES_CATALOG_DATA":           "RELATED_IMAGE_ODH_MODEL_METADATA_COLLECTION_IMAGE",
		"IMAGES_BENCHMARK_DATA":         "RELATED_IMAGE_ODH_MODEL_PERFORMANCE_DATA_IMAGE",
		"IMAGES_JOBS_ASYNC_UPLOAD":      "RELATED_IMAGE_ODH_MODEL_REGISTRY_JOB_ASYNC_UPLOAD_IMAGE",
		"kube-rbac-proxy":               "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
		"IMAGES_POSTGRES":               "RELATED_IMAGE_POSTGRESQL_16_IMAGE",
	}))
	g.Expect(resolved.Kustomize[0].Params[0].Values).To(HaveKeyWithValue("DEFAULT_CERT", "default-modelregistry-cert"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantODH)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantODH))
	g.Expect(resolved.Kustomize).To(HaveLen(2))
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
						ContextDir: "modelregistry",
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

	g.Expect(os.Setenv("MODEL_IMAGE_ENV", "quay.io/example/model:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("MODEL_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/modelregistry/overlays/odh")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/modelregistry/overlays/odh/params.env",
		[]byte("IMAGE=\nSTATIC=\nRUNTIME=\n"),
	)).To(Succeed())
	g.Expect(kfs.MkdirAll("manifests/modelregistry/overlays/odh/extras")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/modelregistry/overlays/odh/extras/params.env",
		[]byte("RUNTIME=\nSECONDARY=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		Params: []ResolvedParamsTarget{
			{
				File: "manifests/modelregistry/overlays/odh/params.env",
				ReplaceFromEnv: map[string]string{
					"IMAGE": "MODEL_IMAGE_ENV",
				},
				Values: map[string]string{
					"STATIC": "static-value",
				},
			},
			{
				File: "manifests/modelregistry/overlays/odh/extras/params.env",
			},
		},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())
	g.Expect(ApplyRuntimeParams(kfs, items, map[string]string{
		"RUNTIME": "runtime-value",
	})).To(Succeed())

	content, err := kfs.ReadFile("manifests/modelregistry/overlays/odh/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring("IMAGE=quay.io/example/model:latest\n"))
	g.Expect(string(content)).To(ContainSubstring("STATIC=static-value\n"))
	g.Expect(string(content)).To(ContainSubstring("RUNTIME=runtime-value\n"))

	extraContent, err := kfs.ReadFile("manifests/modelregistry/overlays/odh/extras/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(extraContent)).To(ContainSubstring("RUNTIME=runtime-value\n"))
	g.Expect(string(extraContent)).To(ContainSubstring("SECONDARY=\n"))
}
