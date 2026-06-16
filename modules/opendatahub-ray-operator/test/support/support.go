package support

import "os"

const (
	DefaultOperatorNamespace        = "opendatahub-ray-system"
	DefaultIntegrationTestNamespace = "integration-test"
	DefaultPlatformName             = "OpenDataHub"
	DefaultPlatformVersion          = "0.1.0"
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

func PlatformName() string {
	if v := os.Getenv("TEST_PLATFORM_TYPE"); v != "" {
		return v
	}
	return DefaultPlatformName
}

func PlatformVersion() string {
	if v := os.Getenv("TEST_PLATFORM_VERSION"); v != "" {
		return v
	}
	return DefaultPlatformVersion
}
