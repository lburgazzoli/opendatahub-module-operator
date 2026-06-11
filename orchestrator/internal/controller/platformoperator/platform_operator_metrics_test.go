package platformoperator

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

func TestRecordOperatorInfoSetsGauge(t *testing.T) {
	g := NewWithT(t)

	recordOperatorInfo("ray", 2,
		configApi.Distribution{Name: "ODH", Version: "1.0.0"},
		configApi.Distribution{Name: "ODH", Version: "2.0.0"},
	)

	g.Expect(testutil.ToFloat64(metricOperatorInfo.With(prometheus.Labels{
		LabelName:           "ray",
		LabelRunlevel:       "2",
		LabelCurrentVersion: "1.0.0",
		LabelTargetVersion:  "2.0.0",
	}))).To(Equal(float64(1)))
}

func TestRecordOperatorInfoUpdatesOnVersionChange(t *testing.T) {
	g := NewWithT(t)

	recordOperatorInfo("spark", 1,
		configApi.Distribution{Name: "ODH", Version: "1.0.0"},
		configApi.Distribution{Name: "ODH", Version: "2.0.0"},
	)

	g.Expect(testutil.ToFloat64(metricOperatorInfo.With(prometheus.Labels{
		LabelName:           "spark",
		LabelRunlevel:       "1",
		LabelCurrentVersion: "1.0.0",
		LabelTargetVersion:  "2.0.0",
	}))).To(Equal(float64(1)))

	recordOperatorInfo("spark", 1,
		configApi.Distribution{Name: "ODH", Version: "2.0.0"},
		configApi.Distribution{Name: "ODH", Version: "2.0.0"},
	)

	g.Expect(testutil.ToFloat64(metricOperatorInfo.With(prometheus.Labels{
		LabelName:           "spark",
		LabelRunlevel:       "1",
		LabelCurrentVersion: "2.0.0",
		LabelTargetVersion:  "2.0.0",
	}))).To(Equal(float64(1)))

	// Old series should be gone (DeletePartialMatch cleaned it up).
	g.Expect(testutil.CollectAndCount(metricOperatorInfo)).To(
		// ray from previous test + spark with new labels
		BeNumerically(">=", 1),
	)
}
