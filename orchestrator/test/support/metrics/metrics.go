package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// GaugeValue returns the current value of a gauge registered under the given name.
func GaugeValue(name string) (float64, error) {
	mf, err := gatherMetric(name)
	if err != nil {
		return 0, err
	}

	if len(mf.GetMetric()) == 0 {
		return 0, fmt.Errorf("metric %q has no samples", name)
	}

	return mf.GetMetric()[0].GetGauge().GetValue(), nil
}

// GaugeVecValue returns the current value of a gauge vec entry with the given labels.
func GaugeVecValue(name string, labels prometheus.Labels) (float64, error) {
	mf, err := gatherMetric(name)
	if err != nil {
		return 0, err
	}

	for _, m := range mf.GetMetric() {
		if matchLabels(m.GetLabel(), labels) {
			return m.GetGauge().GetValue(), nil
		}
	}

	return 0, fmt.Errorf("metric %q: no series matching labels %v", name, labels)
}

func gatherMetric(name string) (*dto.MetricFamily, error) {
	families, err := metrics.Registry.Gather()
	if err != nil {
		return nil, fmt.Errorf("gathering metrics: %w", err)
	}

	for _, f := range families {
		if f.GetName() == name {
			return f, nil
		}
	}

	return nil, fmt.Errorf("metric %q not found in registry", name)
}

func matchLabels(pairs []*dto.LabelPair, labels prometheus.Labels) bool {
	if len(pairs) != len(labels) {
		return false
	}

	for _, p := range pairs {
		v, ok := labels[p.GetName()]
		if !ok || v != p.GetValue() {
			return false
		}
	}

	return true
}
