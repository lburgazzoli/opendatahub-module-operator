package support

import "os"

const (
	DefaultOperatorNamespace        = "opendatahub-trustyai-system"
	DefaultIntegrationTestNamespace = "integration-test"
)

// OperatorNamespace returns the namespace where the module operator is deployed.
// OPERATOR_NAMESPACE takes precedence, then HELM_NAMESPACE for Makefile compatibility.
func OperatorNamespace() string {
	if namespace := os.Getenv("OPERATOR_NAMESPACE"); namespace != "" {
		return namespace
	}

	if namespace := os.Getenv("HELM_NAMESPACE"); namespace != "" {
		return namespace
	}

	return DefaultOperatorNamespace
}

// HelmNamespace is an alias for OperatorNamespace kept for older call sites.
func HelmNamespace() string {
	return OperatorNamespace()
}

func IntegrationTestNamespace() string {
	if namespace := os.Getenv("INTEGRATION_TEST_NAMESPACE"); namespace != "" {
		return namespace
	}

	return DefaultIntegrationTestNamespace
}
