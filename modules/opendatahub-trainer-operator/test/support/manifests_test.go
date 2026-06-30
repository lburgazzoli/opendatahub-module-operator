package support

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

const testManifestPath = "manifests/example.yaml"

const createConfigMapManifest = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: example
  namespace: test-ns
data:
  key: value
`

const updateConfigMapManifest = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: example
  namespace: test-ns
data:
  key: new
`

func TestApplyManifestFromFileCreatesObject(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	scheme := newManifestTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	manifestPath := writeManifestTestFile(t, createConfigMapManifest)

	err := ApplyManifestFromFile(ctx, cli, manifestPath)

	g.Expect(err).NotTo(HaveOccurred())

	stored := &corev1.ConfigMap{}
	err = cli.Get(ctx, client.ObjectKey{Name: "example", Namespace: "test-ns"}, stored)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stored.Data).To(HaveKeyWithValue("key", "value"))
}

func TestApplyManifestFromFSUpdatesExistingObject(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	scheme := newManifestTestScheme(t)
	existing := &corev1.ConfigMap{}
	existing.Name = "example"
	existing.Namespace = "test-ns"
	existing.Data = map[string]string{"key": "old"}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	manifests := fstest.MapFS{
		testManifestPath: {
			Data: []byte(updateConfigMapManifest),
		},
	}

	err := ApplyManifestFromFS(ctx, cli, manifests, testManifestPath)

	g.Expect(err).NotTo(HaveOccurred())

	stored := &corev1.ConfigMap{}
	err = cli.Get(ctx, client.ObjectKey{Name: "example", Namespace: "test-ns"}, stored)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stored.Data).To(HaveKeyWithValue("key", "new"))
}

func newManifestTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding corev1 to scheme: %v", err)
	}

	return scheme
}

func writeManifestTestFile(t *testing.T, content string) string {
	t.Helper()

	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}

	return manifestPath
}
