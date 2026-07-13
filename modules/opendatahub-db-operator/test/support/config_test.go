package support

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster"
)

func TestLoadConfigUsesDefaults(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("ODH_MODULE_OPERATOR_TEST_CLUSTER_TYPE", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_TIMEOUT", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_POLLING_INTERVAL", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_CONSISTENTLY_POLLING_INTERVAL", "")

	cfg, err := LoadConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(*cfg).To(Equal(Config{
		Cluster: ClusterConfig{
			Type: DefaultClusterType,
		},
		Gomega: GomegaConfig{
			EventuallyTimeout:           DefaultEventuallyTimeout,
			EventuallyPollingInterval:   DefaultEventuallyPollingInterval,
			ConsistentlyPollingInterval: DefaultConsistentlyPollingInterval,
		},
	}))
}

func TestLoadConfigUsesEnvironmentOverrides(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("ODH_MODULE_OPERATOR_TEST_CLUSTER_TYPE", "external")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_TIMEOUT", "45s")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_POLLING_INTERVAL", "500ms")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_CONSISTENTLY_POLLING_INTERVAL", "750ms")

	cfg, err := LoadConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(*cfg).To(Equal(Config{
		Cluster: ClusterConfig{
			Type: cluster.TypeExternal,
		},
		Gomega: GomegaConfig{
			EventuallyTimeout:           45 * time.Second,
			EventuallyPollingInterval:   500 * time.Millisecond,
			ConsistentlyPollingInterval: 750 * time.Millisecond,
		},
	}))
}

func TestOperatorNamespaceUsesEnvironmentOverride(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("OPERATOR_NAMESPACE", "custom-namespace")

	g.Expect(OperatorNamespace()).To(Equal("custom-namespace"))
}

func TestOperatorNamespaceFallsBackToDefault(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("OPERATOR_NAMESPACE", "")

	g.Expect(OperatorNamespace()).To(Equal(DefaultOperatorNamespace))
}

func TestIntegrationTestNamespaceUsesEnvironmentOverride(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("INTEGRATION_TEST_NAMESPACE", "custom-integration")

	g.Expect(IntegrationTestNamespace()).To(Equal("custom-integration"))
}

func TestIntegrationTestNamespaceFallsBackToDefault(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("INTEGRATION_TEST_NAMESPACE", "")

	g.Expect(IntegrationTestNamespace()).To(Equal(DefaultIntegrationTestNamespace))
}
