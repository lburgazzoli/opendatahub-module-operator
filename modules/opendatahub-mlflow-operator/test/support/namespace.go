package support

import "os"

const DefaultHelmNamespace = "opendatahub-mlflow-operator-system"

func HelmNamespace() string {
	namespace := os.Getenv("HELM_NAMESPACE")
	if namespace != "" {
		return namespace
	}

	return DefaultHelmNamespace
}
