package db

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestServiceRefForHost(t *testing.T) {
	t.Run("accepts short svc host", func(t *testing.T) {
		g := NewWithT(t)
		serviceName, namespace, ok := serviceRefForHost("postgres.ns.svc")
		g.Expect(ok).To(BeTrue())
		g.Expect(serviceName).To(Equal("postgres"))
		g.Expect(namespace).To(Equal("ns"))
	})

	t.Run("accepts cluster local host", func(t *testing.T) {
		g := NewWithT(t)
		serviceName, namespace, ok := serviceRefForHost("postgres.ns.svc.cluster.local")
		g.Expect(ok).To(BeTrue())
		g.Expect(serviceName).To(Equal("postgres"))
		g.Expect(namespace).To(Equal("ns"))
	})

	t.Run("rejects non service host", func(t *testing.T) {
		g := NewWithT(t)
		_, _, ok := serviceRefForHost("db.example.test")
		g.Expect(ok).To(BeFalse())
	})
}
