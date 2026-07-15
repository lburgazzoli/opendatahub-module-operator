package kind

import (
	"strings"
)

func purgeStaleClusters(
	provider interface {
		List() ([]string, error)
		Delete(name string, explicitKubeconfigPath string) error
	},
	prefix string,
	logFn func(format string, args ...any),
) {
	if provider == nil || prefix == "" {
		return
	}

	names, err := provider.List()
	if err != nil {
		logFn("failed to list stale kind clusters for prefix %q: %v", prefix, err)
		return
	}

	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			if err := provider.Delete(name, ""); err != nil {
				logFn("failed to delete stale kind cluster %q: %v", name, err)
			}
		}
	}
}
