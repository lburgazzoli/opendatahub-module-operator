package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("trainer"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("rhoai"))
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/trainer/rhoai/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"odh-kubeflow-trainer-controller-image",
		"RELATED_IMAGE_ODH_TRAINER_IMAGE",
	))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"odh-th06-cuda130-torch210-py312-image",
		"RELATED_IMAGE_ODH_TH06_CUDA130_TORCH210_PY312_IMAGE",
	))
	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantRhoai)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantRhoai))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("rhoai"))
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
						ContextDir: "trainer",
						SourcePath: "rhoai",
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

	g.Expect(os.Setenv("TRAINER_OPERATOR_IMAGE_ENV", "quay.io/example/trainer:latest")).To(Succeed())
	g.Expect(os.Setenv("TRAINER_TH06_CPU_IMAGE_ENV", "quay.io/example/trainer-th06-cpu:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("TRAINER_OPERATOR_IMAGE_ENV")
		_ = os.Unsetenv("TRAINER_TH06_CPU_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/trainer/rhoai")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/trainer/rhoai/params.env",
		[]byte("odh-kubeflow-trainer-controller-image=\nodh-th06-cpu-torch210-py312-image=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		Params: []ResolvedParamsTarget{{
			File: "manifests/trainer/rhoai/params.env",
			ReplaceFromEnv: map[string]string{
				"odh-kubeflow-trainer-controller-image": "TRAINER_OPERATOR_IMAGE_ENV",
				"odh-th06-cpu-torch210-py312-image":     "TRAINER_TH06_CPU_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())

	content, err := kfs.ReadFile("manifests/trainer/rhoai/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring(
		"odh-kubeflow-trainer-controller-image=quay.io/example/trainer:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring(
		"odh-th06-cpu-torch210-py312-image=quay.io/example/trainer-th06-cpu:latest\n",
	))
}
