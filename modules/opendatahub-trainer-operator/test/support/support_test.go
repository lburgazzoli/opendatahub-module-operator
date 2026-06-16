package support

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestOperatorNamespaceUsesEnvironmentOverride(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("OPERATOR_NAMESPACE", "custom-operator-ns")
	t.Setenv("HELM_NAMESPACE", "helm-ns")

	g.Expect(OperatorNamespace()).To(Equal("custom-operator-ns"))
}

func TestOperatorNamespaceFallsBackToHelmNamespace(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("OPERATOR_NAMESPACE", "")
	t.Setenv("HELM_NAMESPACE", "helm-ns")

	g.Expect(OperatorNamespace()).To(Equal("helm-ns"))
}

func TestOperatorNamespaceFallsBackToDefault(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("OPERATOR_NAMESPACE", "")
	t.Setenv("HELM_NAMESPACE", "")

	g.Expect(OperatorNamespace()).To(Equal(DefaultOperatorNamespace))
}
