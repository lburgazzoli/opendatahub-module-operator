package support

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestLoadGomegaConfigUsesDefaults(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("ODH_MODULE_OPERATOR_TEST_EVENTUALLY_TIMEOUT", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_EVENTUALLY_POLLING_INTERVAL", "")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_CONSISTENTLY_POLLING_INTERVAL", "")

	cfg, err := LoadGomegaConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(*cfg).To(Equal(GomegaConfig{
		EventuallyTimeout:           DefaultEventuallyTimeout,
		EventuallyPollingInterval:   DefaultEventuallyPollingInterval,
		ConsistentlyPollingInterval: DefaultConsistentlyPollingInterval,
	}))
}

func TestLoadGomegaConfigUsesEnvironmentOverrides(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("ODH_MODULE_OPERATOR_TEST_EVENTUALLY_TIMEOUT", "45s")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_EVENTUALLY_POLLING_INTERVAL", "500ms")
	t.Setenv("ODH_MODULE_OPERATOR_TEST_CONSISTENTLY_POLLING_INTERVAL", "750ms")

	cfg, err := LoadGomegaConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(*cfg).To(Equal(GomegaConfig{
		EventuallyTimeout:           45 * time.Second,
		EventuallyPollingInterval:   500 * time.Millisecond,
		ConsistentlyPollingInterval: 750 * time.Millisecond,
	}))
}
