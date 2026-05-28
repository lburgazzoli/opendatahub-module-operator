package support

import "os"

const (
	DefaultOperatorNamespace        = "opendatahub-workbenches-system"
	DefaultIntegrationTestNamespace = "opendatahub-workbenches-integration-tests"
)

func OperatorNamespace() string {
	if namespace := os.Getenv("OPERATOR_NAMESPACE"); namespace != "" {
		return namespace
	}

	return DefaultOperatorNamespace
}

func IntegrationTestNamespace() string {
	if namespace := os.Getenv("INTEGRATION_TEST_NAMESPACE"); namespace != "" {
		return namespace
	}

	return DefaultIntegrationTestNamespace
}

// HelmNamespace returns the operator namespace used by e2e and Helm deploy targets.
func HelmNamespace() string {
	return OperatorNamespace()
}
