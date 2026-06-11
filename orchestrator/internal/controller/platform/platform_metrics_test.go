package platform

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordRunlevelSetsGauge(t *testing.T) {
	g := NewWithT(t)

	recordRunlevel(3)
	g.Expect(testutil.ToFloat64(metricRunlevel)).To(Equal(float64(3)))

	recordRunlevel(5)
	g.Expect(testutil.ToFloat64(metricRunlevel)).To(Equal(float64(5)))
}

func TestRecordAdminAcksSetsPendingAndSatisfied(t *testing.T) {
	g := NewWithT(t)

	required := map[string]adminAckRequirement{
		"ack.alpha": {Name: "ack.alpha", Modules: []string{"alpha"}},
		"ack.beta":  {Name: "ack.beta", Modules: []string{"beta"}},
	}
	unsatisfied := []unsatisfiedAdminAck{
		{Name: "ack.alpha", Modules: []string{"alpha"}, Value: "false"},
	}

	recordAdminAcks(required, unsatisfied)

	g.Expect(testutil.ToFloat64(metricAdminAckPending.WithLabelValues("ack.alpha"))).To(Equal(float64(1)))
	g.Expect(testutil.ToFloat64(metricAdminAckPending.WithLabelValues("ack.beta"))).To(Equal(float64(0)))
}

func TestRecordAdminAcksAllSatisfied(t *testing.T) {
	g := NewWithT(t)

	required := map[string]adminAckRequirement{
		"ack.alpha": {Name: "ack.alpha", Modules: []string{"alpha"}},
		"ack.beta":  {Name: "ack.beta", Modules: []string{"beta"}},
	}

	recordAdminAcks(required, nil)

	g.Expect(testutil.ToFloat64(metricAdminAckPending.WithLabelValues("ack.alpha"))).To(Equal(float64(0)))
	g.Expect(testutil.ToFloat64(metricAdminAckPending.WithLabelValues("ack.beta"))).To(Equal(float64(0)))
}
