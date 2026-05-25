package support

import "testing"

func TestHelmNamespaceUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("HELM_NAMESPACE", "custom-namespace")

	if got := HelmNamespace(); got != "custom-namespace" {
		t.Fatalf("HelmNamespace() = %q, want %q", got, "custom-namespace")
	}
}

func TestHelmNamespaceFallsBackToDefault(t *testing.T) {
	t.Setenv("HELM_NAMESPACE", "")

	if got := HelmNamespace(); got != DefaultHelmNamespace {
		t.Fatalf("HelmNamespace() = %q, want %q", got, DefaultHelmNamespace)
	}
}
