package external

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

func TestNewFromConfigRejectsNil(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	cluster, err := NewFromConfig(nil, nil)

	g.Expect(err).To(HaveOccurred())
	g.Expect(cluster).To(BeNil())
}

func TestNewFromConfigReturnsCopy(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	cfg := &rest.Config{Host: "https://cluster.example.invalid"}

	cluster, err := NewFromConfig(cfg, nil)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cluster.Config()).NotTo(BeNil())
	g.Expect(cluster.Config()).NotTo(BeIdenticalTo(cfg))
	g.Expect(cluster.Config().Host).To(Equal(cfg.Host))
	g.Expect(cluster.Client()).NotTo(BeNil())
}
