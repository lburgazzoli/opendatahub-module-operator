package cluster

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"k8s.io/client-go/rest"
)

func TestNewRejectsUnknownType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	cluster, err := New(context.Background(), Type("unknown"))

	g.Expect(err).To(HaveOccurred())
	g.Expect(cluster).To(BeNil())
}

func TestNewExternalFromConfigRejectsNil(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	cluster, err := NewExternalFromConfig(nil)

	g.Expect(err).To(HaveOccurred())
	g.Expect(cluster).To(BeNil())
}

func TestNewExternalFromConfigReturnsCopy(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	cfg := &rest.Config{Host: "https://cluster.example.invalid"}

	cluster, err := NewExternalFromConfig(cfg)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cluster.Config()).NotTo(BeNil())
	g.Expect(cluster.Config()).NotTo(BeIdenticalTo(cfg))
	g.Expect(cluster.Config().Host).To(Equal(cfg.Host))
	g.Expect(cluster.Client()).NotTo(BeNil())
	g.Expect(cluster.Scheme()).NotTo(BeNil())
}

func TestDefaultK3sOptionsUseDefaultImage(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	options := defaultK3sOptions()

	g.Expect(options.Image).To(Equal(DefaultK3sImage))
	g.Expect(options.Customizers).To(BeEmpty())
}

func TestWithK3sImageOverridesDefault(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	options := defaultK3sOptions()

	WithK3sImage("rancher/k3s:v1.33.1-k3s1")(&options)

	g.Expect(options.Image).To(Equal("rancher/k3s:v1.33.1-k3s1"))
}

func TestWithContainerCustomizerAppendsCustomizer(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	options := defaultK3sOptions()
	customizer := testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{})

	WithContainerCustomizer(customizer)(&options)

	g.Expect(options.Customizers).To(HaveLen(1))
}
