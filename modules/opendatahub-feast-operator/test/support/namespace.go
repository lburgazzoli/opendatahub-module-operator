package support

import "os"

const DefaultHelmNamespace = "opendatahub-feast-operator-system"

func HelmNamespace() string {
	namespace := os.Getenv("HELM_NAMESPACE")
	if namespace != "" {
		return namespace
	}

	return DefaultHelmNamespace
}
