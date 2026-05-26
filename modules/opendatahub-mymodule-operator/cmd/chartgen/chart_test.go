package chartgen

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestTransformDeploymentAlwaysPullPolicy(t *testing.T) {
	g := NewWithT(t)

	raw := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        image: quay.io/example/controller:dev
        imagePullPolicy: IfNotPresent
`

	transformed := replaceImageField(raw)
	g.Expect(transformed).To(ContainSubstring(`image: "{{ include "chart.imageRef" . }}"`))
	g.Expect(transformed).To(ContainSubstring("imagePullPolicy: Always"))
	g.Expect(transformed).NotTo(ContainSubstring(".Values.image.pullPolicy"))
}

func TestWriteValuesYAMLOmitsPullPolicy(t *testing.T) {
	g := NewWithT(t)

	path := filepath.Join(t.TempDir(), "values.yaml")
	err := WriteValuesYAML(DefaultValues(), path)
	g.Expect(err).NotTo(HaveOccurred())

	data, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(data)).NotTo(ContainSubstring("pullPolicy:"))
}

func TestWriteValuesSchemaOmitsPullPolicy(t *testing.T) {
	g := NewWithT(t)

	path := filepath.Join(t.TempDir(), "values.schema.json")
	err := WriteValuesSchema(path)
	g.Expect(err).NotTo(HaveOccurred())

	data, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(data)).NotTo(ContainSubstring(`"pullPolicy"`))
}
