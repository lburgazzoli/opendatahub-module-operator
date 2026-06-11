package platformoperator

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

const (
	MetricPlatformOperatorInfo = "odh_platform_operator_info"

	LabelName           = "name"
	LabelRunlevel       = "runlevel"
	LabelCurrentVersion = "current_version"
	LabelTargetVersion  = "target_version"
)

var metricOperatorInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: MetricPlatformOperatorInfo,
		Help: "Info metric for each PlatformOperator (value always 1).",
	},
	[]string{LabelName, LabelRunlevel, LabelCurrentVersion, LabelTargetVersion},
)

func init() {
	metrics.Registry.MustRegister(
		metricOperatorInfo,
	)
}

func recordOperatorInfo(
	name string,
	runlevel int,
	current configApi.Distribution,
	target configApi.Distribution,
) {
	metricOperatorInfo.DeletePartialMatch(prometheus.Labels{LabelName: name})
	metricOperatorInfo.WithLabelValues(
		name,
		fmt.Sprintf("%d", runlevel),
		current.Version,
		target.Version,
	).Set(1)
}
