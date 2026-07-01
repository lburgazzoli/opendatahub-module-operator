package module

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/assets"
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
	g.Expect(resolved.Kustomize[0].ManifestInfo.ContextDir).To(Equal("ray"))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("openshift"))
	g.Expect(resolved.Kustomize[0].Params).To(HaveLen(1))
	g.Expect(resolved.Kustomize[0].Params[0].File).To(Equal("manifests/ray/openshift/params.env"))
	g.Expect(resolved.Kustomize[0].Params[0].ReplaceFromEnv).To(Equal(map[string]string{
		"odh-kuberay-operator-controller-image": "RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE",
	}))
	g.Expect(resolved.Templates).To(HaveLen(1))
	g.Expect(resolved.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestLoadVariant(t *testing.T) {
	g := NewWithT(t)

	resolved, err := LoadVariant(assets.Manifests, DescriptorPath, VariantRhoai)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Name).To(Equal(VariantRhoai))
	g.Expect(resolved.Kustomize[0].ManifestInfo.SourcePath).To(Equal("openshift"))
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
						ContextDir: "ray",
						SourcePath: "openshift",
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

	g.Expect(os.Setenv("RAY_OPERATOR_IMAGE_ENV", "quay.io/example/ray-operator:latest")).To(Succeed())
	t.Cleanup(func() {
		_ = os.Unsetenv("RAY_OPERATOR_IMAGE_ENV")
	})

	kfs := filesys.MakeFsInMemory()
	g.Expect(kfs.MkdirAll("manifests/ray/openshift")).To(Succeed())
	g.Expect(kfs.WriteFile(
		"manifests/ray/openshift/params.env",
		[]byte("namespace=\nodh-kuberay-operator-controller-image=\n"),
	)).To(Succeed())

	items := []ResolvedKustomizeItem{{
		Params: []ResolvedParamsTarget{{
			File: "manifests/ray/openshift/params.env",
			ReplaceFromEnv: map[string]string{
				"odh-kuberay-operator-controller-image": "RAY_OPERATOR_IMAGE_ENV",
			},
		}},
	}}

	g.Expect(ApplyStaticParams(kfs, items)).To(Succeed())
	g.Expect(ApplyRuntimeParams(kfs, items, map[string]string{
		"namespace": "test-ns",
	})).To(Succeed())

	content, err := kfs.ReadFile("manifests/ray/openshift/params.env")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring(
		"odh-kuberay-operator-controller-image=quay.io/example/ray-operator:latest\n",
	))
	g.Expect(string(content)).To(ContainSubstring("namespace=test-ns\n"))
}
