package cluster

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

func TestNewRejectsUnknownType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	cluster, err := New(context.Background(), &support.Config{
		Cluster: support.ClusterConfig{
			Type: support.ClusterType("unknown"),
		},
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(cluster).To(BeNil())
}
