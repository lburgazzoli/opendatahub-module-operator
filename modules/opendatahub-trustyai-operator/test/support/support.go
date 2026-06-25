package support

import "os"

const (
	DefaultOperatorNamespace        = "odh-trustyai-system"
	DefaultIntegrationTestNamespace = "odh-trustyai-integration"
	DefaultPlatformVersion          = "0.1.0"
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

func PlatformVersion() string {
	if v := os.Getenv("TEST_PLATFORM_VERSION"); v != "" {
		return v
	}
	return DefaultPlatformVersion
}
