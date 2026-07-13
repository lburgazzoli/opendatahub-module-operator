package k3s

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
)

func TestDefaultOptionsUseDefaultImage(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	options := defaultOptions()

	g.Expect(options.Image).To(Equal(defaultImage))
	g.Expect(options.Customizers).To(BeEmpty())
}

func TestWithImageOverridesDefault(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	options := defaultOptions()

	WithImage("rancher/k3s:v1.33.1-k3s1")(&options)

	g.Expect(options.Image).To(Equal("rancher/k3s:v1.33.1-k3s1"))
}

func TestWithContainerCustomizerAppendsCustomizer(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	options := defaultOptions()
	customizer := testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{})

	WithContainerCustomizer(customizer)(&options)

	g.Expect(options.Customizers).To(HaveLen(1))
}
