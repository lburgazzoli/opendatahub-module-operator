package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("mlflowoperator"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].SkipRender).To(BeFalse())
	g.Expect(resolved.Kustomize[0].Params).To(BeEmpty())

	g.Expect(resolved.Kustomize[1].ManifestInfo.SourcePath).To(Equal("base"))
	g.Expect(resolved.Kustomize[1].SkipRender).To(BeTrue())
	g.Expect(resolved.Kustomize[1].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[1].Params[0].File).To(Equal("manifests/mlflowoperator/base/params.env"))
	g.Expect(resolved.Kustomize[1].Params[0].ReplaceFromEnv).To(Equal(map[string]string{
		"MLFLOW_IMAGE":          "RELATED_IMAGE_ODH_MLFLOW_IMAGE",
		"MLFLOW_OPERATOR_IMAGE": "RELATED_IMAGE_ODH_MLFLOW_OPERATOR_IMAGE",
		"KUBE_AUTH_PROXY_IMAGE": "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
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
						ContextDir: "mlflowoperator",
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

	g.Expect(os.Setenv("MLFLOW_IMAGE_ENV", "quay.io/example/mlflow:latest")).To(Succeed())
	g.Expect(os.Setenv("MLFLOW_OPERATOR_IMAGE_ENV", "quay.io/example/mlflow-operator:latest")).To(Succeed())
	g.Expect(os.Setenv("KUBE_RBAC_PROXY_IMAGE_ENV", "quay.io/example/kube-rbac-proxy:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("MLFLOW_IMAGE_ENV")
		_ = os.Unsetenv("MLFLOW_OPERATOR_IMAGE_ENV")
		_ = os.Unsetenv("KUBE_RBAC_PROXY_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/mlflowoperator/base")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/mlflowoperator/base/params.env",
		[]byte("MLFLOW_IMAGE=\nMLFLOW_OPERATOR_IMAGE=\nKUBE_AUTH_PROXY_IMAGE=\nmlflow-url=\nsection-title=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		SkipRender: true,
		Params: []ResolvedParamsTarget{{
			File: "manifests/mlflowoperator/base/params.env",
			ReplaceFromEnv: map[string]string{
				"MLFLOW_IMAGE":          "MLFLOW_IMAGE_ENV",
				"MLFLOW_OPERATOR_IMAGE": "MLFLOW_OPERATOR_IMAGE_ENV",
				"KUBE_AUTH_PROXY_IMAGE": "KUBE_RBAC_PROXY_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())
	g.Expect(ApplyRuntimeParams(kfs, items, map[string]string{
		"mlflow-url":    "https://example.apps.cluster/",
		"section-title": "OpenShift Open Data Hub",
	})).To(Succeed())

	content, err := kfs.ReadFile("manifests/mlflowoperator/base/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring("MLFLOW_IMAGE=quay.io/example/mlflow:latest\n"))
	g.Expect(string(content)).To(ContainSubstring(
		"MLFLOW_OPERATOR_IMAGE=quay.io/example/mlflow-operator:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring(
		"KUBE_AUTH_PROXY_IMAGE=quay.io/example/kube-rbac-proxy:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring("mlflow-url=https://example.apps.cluster/\n"))
	g.Expect(string(content)).To(ContainSubstring("section-title=OpenShift Open Data Hub\n"))
}
