package helm

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestWithValuesMergesNestedMaps(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	options := InstallOptions{
		Values: map[string]any{
			"operator": map[string]any{
				"replicas": 1,
			},
		},
	}

	WithValues(map[string]any{
		"operator": map[string]any{
			"image": map[string]any{
				"ref": "quay.io/example/operator:test",
			},
		},
		"platform": map[string]any{
			"type":    "OpenDataHub",
			"version": "0.1.0",
		},
	}).ApplyTo(&options)

	g.Expect(options.Values).To(Equal(map[string]any{
		"operator": map[string]any{
			"replicas": 1,
			"image": map[string]any{
				"ref": "quay.io/example/operator:test",
			},
		},
		"platform": map[string]any{
			"type":    "OpenDataHub",
			"version": "0.1.0",
		},
	}))
}

func TestGetValuesMatchesHelmSetSemantics(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	options := InstallOptions{}

	WithValue("operator.image.ref", "quay.io/example/operator:test").ApplyTo(&options)
	WithValue("platform.type", "OpenDataHub").ApplyTo(&options)
	WithValue("platform.version", "0.1.0").ApplyTo(&options)

	values, err := options.GetValues()

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(values).To(Equal(map[string]any{
		"operator": map[string]any{
			"image": map[string]any{
				"ref": "quay.io/example/operator:test",
			},
		},
		"platform": map[string]any{
			"type":    "OpenDataHub",
			"version": "0.1.0",
		},
	}))
}
