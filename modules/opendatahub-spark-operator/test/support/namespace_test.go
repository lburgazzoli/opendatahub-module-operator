package support

import "testing"

func TestOperatorNamespaceUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("OPERATOR_NAMESPACE", "custom-namespace")

	if got := OperatorNamespace(); got != "custom-namespace" {
		t.Fatalf("OperatorNamespace() = %q, want %q", got, "custom-namespace")
	}
}

func TestOperatorNamespaceFallsBackToDefault(t *testing.T) {
	t.Setenv("OPERATOR_NAMESPACE", "")

	if got := OperatorNamespace(); got != DefaultOperatorNamespace {
		t.Fatalf("OperatorNamespace() = %q, want %q", got, DefaultOperatorNamespace)
	}
}

func TestIntegrationTestNamespaceUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("INTEGRATION_TEST_NAMESPACE", "custom-integration")

	if got := IntegrationTestNamespace(); got != "custom-integration" {
		t.Fatalf("IntegrationTestNamespace() = %q, want %q", got, "custom-integration")
	}
}

func TestIntegrationTestNamespaceFallsBackToDefault(t *testing.T) {
	t.Setenv("INTEGRATION_TEST_NAMESPACE", "")

	if got := IntegrationTestNamespace(); got != DefaultIntegrationTestNamespace {
		t.Fatalf("IntegrationTestNamespace() = %q, want %q", got, DefaultIntegrationTestNamespace)
	}
}
