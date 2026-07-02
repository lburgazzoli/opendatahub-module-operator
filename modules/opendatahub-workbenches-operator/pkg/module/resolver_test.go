package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/assets"
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
	g.Expect(resolved.Kustomize).To(HaveLen(4))
	g.Expect(resolved.Kustomize[0].ManifestInfo.Path).To(Equal("manifests"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("workbenches/odh-notebook-controller"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("base"))
	g.Expect(resolved.Kustomize[0].SkipRender).To(BeFalse())
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal(
		"manifests/workbenches/odh-notebook-controller/base/params.env",
	))
	g.Expect(resolved.Kustomize[1].ManifestInfo.SourcePath).To(Equal("overlays/openshift"))
	g.Expect(resolved.Kustomize[2].ManifestInfo.SourcePath).To(Equal("odh/overlays/additional"))
	g.Expect(resolved.Kustomize[2].Params).To(BeEmpty())
	g.Expect(resolved.Kustomize[3].ManifestInfo.SourcePath).To(Equal("odh/base"))
	g.Expect(resolved.Kustomize[3].SkipRender).To(BeTrue())
	g.Expect(resolved.Kustomize[3].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[3].Params[0].File).To(Equal(
		"manifests/workbenches/notebooks/odh/base/params-latest.env",
	))
	g.Expect(resolved.Kustomize[3].Params[0].ReplaceFromEnv).To(HaveKeyWithValue(
		"odh-workbench-jupyter-minimal-cpu-py312-ubi9-n",
		"RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_MINIMAL_CPU_PY312_IMAGE",
	))
	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantRhoai)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantRhoai))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("base"))
	g.Expect(resolved.Kustomize[2].ManifestInfo.SourcePath).To(Equal("odh/overlays/additional"))
	g.Expect(resolved.Kustomize[3].ManifestInfo.SourcePath).To(Equal("odh/base"))
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
						ContextDir: "workbenches/odh-notebook-controller",
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

	g.Expect(os.Setenv("NOTEBOOK_CONTROLLER_IMAGE_ENV", "quay.io/example/notebook-controller:latest")).To(Succeed())
	g.Expect(os.Setenv("NOTEBOOK_IMAGE_ENV", "quay.io/example/notebook-image:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("NOTEBOOK_CONTROLLER_IMAGE_ENV")
		_ = os.Unsetenv("NOTEBOOK_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/workbenches/odh-notebook-controller/base")).To(Succeed())
	g.Expect(kfs.MkdirAll("manifests/workbenches/notebooks/odh/base")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/workbenches/odh-notebook-controller/base/params.env",
		[]byte("odh-notebook-controller-image=\ngateway-url=\nsection-title=\nmlflow-enabled=\n"),
	)).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/workbenches/notebooks/odh/base/params-latest.env",
		[]byte("odh-workbench-jupyter-minimal-cpu-py312-ubi9-n=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{
		{
			Params: []ResolvedParamsTarget{{
				File: "manifests/workbenches/odh-notebook-controller/base/params.env",
				ReplaceFromEnv: map[string]string{
					"odh-notebook-controller-image": "NOTEBOOK_CONTROLLER_IMAGE_ENV",
				},
			}},
		},
		{
			SkipRender: true,
			Params: []ResolvedParamsTarget{{
				File: "manifests/workbenches/notebooks/odh/base/params-latest.env",
				ReplaceFromEnv: map[string]string{
					"odh-workbench-jupyter-minimal-cpu-py312-ubi9-n": "NOTEBOOK_IMAGE_ENV",
				},
			}},
		},
	}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())
	g.Expect(ApplyRuntimeParams(kfs, items[:1], map[string]string{
		"gateway-url":    "example.apps.cluster.local",
		"section-title":  "OpenShift Open Data Hub",
		"mlflow-enabled": "true",
	})).To(Succeed())

	controllerContent, err := kfs.ReadFile("manifests/workbenches/odh-notebook-controller/base/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(controllerContent)).To(ContainSubstring(
		"odh-notebook-controller-image=quay.io/example/notebook-controller:latest\n",
	))
	g.Expect(string(controllerContent)).To(ContainSubstring(
		"gateway-url=example.apps.cluster.local\n",
	))
	g.Expect(string(controllerContent)).To(ContainSubstring(
		"section-title=OpenShift Open Data Hub\n",
	))
	g.Expect(string(controllerContent)).To(ContainSubstring("mlflow-enabled=true\n"))

	notebookContent, err := kfs.ReadFile("manifests/workbenches/notebooks/odh/base/params-latest.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(notebookContent)).To(ContainSubstring(
		"odh-workbench-jupyter-minimal-cpu-py312-ubi9-n=quay.io/example/notebook-image:latest\n",
	))
}
