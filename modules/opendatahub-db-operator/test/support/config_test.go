package support

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestLoadConfigUsesDefaults(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("ODH_MODULE_OPERATOR_TEST_CLUSTER_TYPE", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_TIMEOUT", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_POLLING_INTERVAL", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_CONSISTENTLY_POLLING_INTERVAL", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_INSTALL", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_LOGS", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_IMAGE", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_NAMESPACE", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_PLATFORM_TYPE", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_PLATFORM_VERSION", "")

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
		Operator: OperatorConfig{
			Install:         DefaultOperatorInstall,
			Logs:            DefaultOperatorLogs,
			Image:           "",
			Namespace:       DefaultOperatorNamespace,
			PlatformType:    DefaultPlatformType,
			PlatformVersion: DefaultPlatformVersion,
		},
	}))
}

func TestLoadConfigUsesEnvironmentOverrides(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("ODH_MODULE_OPERATOR_TEST_CLUSTER_TYPE", "external")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_TIMEOUT", "45s")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_EVENTUALLY_POLLING_INTERVAL", "500ms")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_GOMEGA_CONSISTENTLY_POLLING_INTERVAL", "750ms")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_INSTALL", "true")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_LOGS", "false")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_IMAGE", "quay.io/example/operator:test")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_NAMESPACE", "custom-operator")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_PLATFORM_TYPE", "RHOAI")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_OPERATOR_PLATFORM_VERSION", "2.22.0")

	cfg, err := LoadConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(*cfg).To(Equal(Config{
		Cluster: ClusterConfig{
			Type: ClusterTypeExternal,
		},
		Gomega: GomegaConfig{
			EventuallyTimeout:           45 * time.Second,
			EventuallyPollingInterval:   500 * time.Millisecond,
			ConsistentlyPollingInterval: 750 * time.Millisecond,
		},
		Operator: OperatorConfig{
			Install:         true,
			Logs:            false,
			Image:           "quay.io/example/operator:test",
			Namespace:       "custom-operator",
			PlatformType:    "RHOAI",
			PlatformVersion: "2.22.0",
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
