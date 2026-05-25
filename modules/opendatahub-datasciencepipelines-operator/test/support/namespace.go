package support

import "os"

const DefaultHelmNamespace = "opendatahub-datasciencepipelines-operator-system"

func HelmNamespace() string {
	namespace := os.Getenv("HELM_NAMESPACE")
	if namespace != "" {
		return namespace
	}

	return DefaultHelmNamespace
}
