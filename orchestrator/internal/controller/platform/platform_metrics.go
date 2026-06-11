package platform

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	MetricPlatformRunlevel        = "odh_platform_runlevel"
	MetricPlatformAdminAckPending = "odh_platform_admin_ack_pending"

	LabelAckName = "ack_name"
)

var (
	metricRunlevel = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: MetricPlatformRunlevel,
			Help: "Current platform runlevel.",
		},
	)

	metricAdminAckPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricPlatformAdminAckPending,
			Help: "Whether an admin acknowledgement is pending (1) or satisfied (0).",
		},
		[]string{LabelAckName},
	)
)

func init() {
	metrics.Registry.MustRegister(
		metricRunlevel,
		metricAdminAckPending,
	)
}

func recordRunlevel(level int) {
	metricRunlevel.Set(float64(level))
}

func recordAdminAcks(required map[string]adminAckRequirement, unsatisfied []unsatisfiedAdminAck) {
	pending := make(map[string]float64, len(unsatisfied))
	for _, ack := range unsatisfied {
		pending[ack.Name] = 1
	}

	for name := range required {
		metricAdminAckPending.WithLabelValues(name).Set(pending[name])
	}
}
