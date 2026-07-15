package kind

import (
	"testing"

	. "github.com/onsi/gomega"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

func TestDefaultOptionsUseSafeDefaults(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	logsDir := t.TempDir()

	options := options{
		NamePrefix:    clusterPrefix,
		CreateOptions: []kindcluster.CreateOption{kindcluster.CreateWithWaitForReady(defaultWaitForReady)},
		LogsDir:       logsDir,
	}

	g.Expect(options.NamePrefix).To(Equal(clusterPrefix))
	g.Expect(options.ProviderOptions).To(BeEmpty())
	g.Expect(options.CreateOptions).To(HaveLen(1))
	g.Expect(options.LogsDir).To(Equal(logsDir))
}

func TestOptionsValidateRejectsEmptyNamePrefix(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	err := (options{}).Validate()

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("name prefix is empty"))
}

func TestWithNodeImageOverridesDefault(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	logsDir := t.TempDir()
	options := options{
		NamePrefix:    clusterPrefix,
		CreateOptions: []kindcluster.CreateOption{kindcluster.CreateWithWaitForReady(defaultWaitForReady)},
		LogsDir:       logsDir,
	}

	WithNodeImage("kindest/node:v1.33.1").applyOption(&options)

	g.Expect(options.CreateOptions).To(HaveLen(2))
}

func TestWithProviderOptionAppendsProviderOption(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	logsDir := t.TempDir()
	options := options{
		NamePrefix:    clusterPrefix,
		CreateOptions: []kindcluster.CreateOption{kindcluster.CreateWithWaitForReady(defaultWaitForReady)},
		LogsDir:       logsDir,
	}

	WithProviderOption(kindcluster.ProviderWithPodman()).applyOption(&options)

	g.Expect(options.ProviderOptions).To(HaveLen(1))
}

func TestWithPurgeOverridesDefault(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	purgeEnabled := true
	logsDir := t.TempDir()
	options := options{
		NamePrefix:    clusterPrefix,
		CreateOptions: []kindcluster.CreateOption{kindcluster.CreateWithWaitForReady(defaultWaitForReady)},
		LogsDir:       logsDir,
		Purge:         &purgeEnabled,
	}

	WithPurge(false).applyOption(&options)

	g.Expect(options.Purge).NotTo(BeNil())
	g.Expect(*options.Purge).To(BeFalse())
}

func TestOptionsPresetAppliesNonZeroFields(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	logsDir := t.TempDir()
	base := options{
		NamePrefix:    clusterPrefix,
		CreateOptions: []kindcluster.CreateOption{kindcluster.CreateWithWaitForReady(defaultWaitForReady)},
		LogsDir:       logsDir,
	}

	options{
		NamePrefix: "custom-",
		CreateOptions: []kindcluster.CreateOption{
			kindcluster.CreateWithNodeImage("kindest/node:v1.33.1"),
			kindcluster.CreateWithRawConfig([]byte("kind: Cluster")),
			kindcluster.CreateWithWaitForReady(45),
		},
		LogsDir: "custom-logs",
	}.applyOption(&base)

	g.Expect(base.NamePrefix).To(Equal("custom-"))
	g.Expect(base.CreateOptions).To(HaveLen(4))
	g.Expect(base.LogsDir).To(Equal("custom-logs"))
}
