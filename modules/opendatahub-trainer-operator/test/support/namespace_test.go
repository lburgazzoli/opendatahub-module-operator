package support

import "testing"

func TestOperatorNamespaceUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("OPERATOR_NAMESPACE", "custom-operator-ns")
	t.Setenv("HELM_NAMESPACE", "helm-ns")

	if got := OperatorNamespace(); got != "custom-operator-ns" {
		t.Fatalf("OperatorNamespace() = %q, want %q", got, "custom-operator-ns")
	}
}

func TestOperatorNamespaceFallsBackToHelmNamespace(t *testing.T) {
	t.Setenv("OPERATOR_NAMESPACE", "")
	t.Setenv("HELM_NAMESPACE", "helm-ns")

	if got := OperatorNamespace(); got != "helm-ns" {
		t.Fatalf("OperatorNamespace() = %q, want %q", got, "helm-ns")
	}
}

func TestOperatorNamespaceFallsBackToDefault(t *testing.T) {
	t.Setenv("OPERATOR_NAMESPACE", "")
	t.Setenv("HELM_NAMESPACE", "")

	if got := OperatorNamespace(); got != DefaultOperatorNamespace {
		t.Fatalf("OperatorNamespace() = %q, want %q", got, DefaultOperatorNamespace)
	}
}
