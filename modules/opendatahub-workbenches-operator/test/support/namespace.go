package support

import "os"

const DefaultHelmNamespace = "opendatahub-workbenches-operator-system"

func HelmNamespace() string {
	namespace := os.Getenv("HELM_NAMESPACE")
	if namespace != "" {
		return namespace
	}

	return DefaultHelmNamespace
}
