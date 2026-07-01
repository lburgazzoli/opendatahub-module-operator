package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("sparkoperator"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/sparkoperator/overlays/odh/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(Equal(map[string]string{
		"SPARK_OPERATOR_CONTROLLER_IMAGE": "RELATED_IMAGE_ODH_SPARK_OPERATOR_IMAGE",
		"SPARK_OPERATOR_WEBHOOK_IMAGE":    "RELATED_IMAGE_ODH_SPARK_OPERATOR_IMAGE",
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
						ContextDir: "sparkoperator",
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

	g.Expect(os.Setenv("SPARK_CONTROLLER_IMAGE_ENV", "quay.io/example/spark-controller:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("SPARK_CONTROLLER_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/sparkoperator/overlays/odh")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/sparkoperator/overlays/odh/params.env",
		[]byte("SPARK_OPERATOR_CONTROLLER_IMAGE=\nSPARK_OPERATOR_WEBHOOK_IMAGE=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		Params: []ResolvedParamsTarget{{
			File: "manifests/sparkoperator/overlays/odh/params.env",
			ReplaceFromEnv: map[string]string{
				"SPARK_OPERATOR_CONTROLLER_IMAGE": "SPARK_CONTROLLER_IMAGE_ENV",
				"SPARK_OPERATOR_WEBHOOK_IMAGE":    "SPARK_CONTROLLER_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())

	content, err := kfs.ReadFile("manifests/sparkoperator/overlays/odh/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring(
		"SPARK_OPERATOR_CONTROLLER_IMAGE=quay.io/example/spark-controller:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring(
		"SPARK_OPERATOR_WEBHOOK_IMAGE=quay.io/example/spark-controller:latest\n",
	))
}
