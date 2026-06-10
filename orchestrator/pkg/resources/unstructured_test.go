package resources

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

func TestDecode(t *testing.T) {
	t.Run("decodes typed object", func(t *testing.T) {
		g := NewWithT(t)
		src := &configApi.Platform{
			ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
			Status:     configApi.PlatformStatus{Runlevel: 2},
		}

		dest, err := Decode[*configApi.Platform](src)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dest.Name).To(Equal(src.Name))
		g.Expect(dest.Status.Runlevel).To(Equal(src.Status.Runlevel))
	})

	t.Run("decodes unstructured object", func(t *testing.T) {
		g := NewWithT(t)
		src := &configApi.Platform{
			TypeMeta: metav1.TypeMeta{
				APIVersion: configApi.GroupVersion.String(),
				Kind:       "Platform",
			},
			ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
			Status:     configApi.PlatformStatus{Runlevel: 3},
		}
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(src)
		g.Expect(err).NotTo(HaveOccurred())

		dest, err := Decode[*configApi.Platform](&unstructured.Unstructured{Object: content})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dest.Name).To(Equal(src.Name))
		g.Expect(dest.Status.Runlevel).To(Equal(src.Status.Runlevel))
	})

	t.Run("rejects unexpected object type", func(t *testing.T) {
		g := NewWithT(t)

		_, err := Decode[*configApi.Platform](&configApi.PlatformOperator{})

		g.Expect(err).To(HaveOccurred())
	})
}

func TestMetadataHelpers(t *testing.T) {
	t.Run("set label initializes map and returns old value", func(t *testing.T) {
		g := NewWithT(t)
		obj := &configApi.Platform{}

		old := SetLabel(obj, "app", "ray")
		g.Expect(old).To(BeEmpty())
		g.Expect(obj.Labels).To(HaveKeyWithValue("app", "ray"))

		old = SetLabel(obj, "app", "spark")
		g.Expect(old).To(Equal("ray"))
		g.Expect(obj.Labels).To(HaveKeyWithValue("app", "spark"))
	})

	t.Run("set annotation initializes map and returns old value", func(t *testing.T) {
		g := NewWithT(t)
		obj := &configApi.Platform{}

		old := SetAnnotation(obj, "config.opendatahub.io/ray", "true")
		g.Expect(old).To(BeEmpty())
		g.Expect(obj.Annotations).To(HaveKeyWithValue("config.opendatahub.io/ray", "true"))

		old = SetAnnotation(obj, "config.opendatahub.io/ray", "false")
		g.Expect(old).To(Equal("true"))
		g.Expect(obj.Annotations).To(HaveKeyWithValue("config.opendatahub.io/ray", "false"))
	})
}

var _ client.Object = (*configApi.Platform)(nil)
