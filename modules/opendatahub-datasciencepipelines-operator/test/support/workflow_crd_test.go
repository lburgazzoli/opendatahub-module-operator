package support

import (
	"context"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testWorkflowCRDName   = "workflows.argoproj.io"
	testManagedLabel      = "testing.opendatahub.io/managed-by"
	testManagedLabelValue = "dsp-e2e"
)

func TestRestoreWorkflowCRDStateRestoresOriginalLabels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cli := newFakeClient(
		t,
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testWorkflowCRDName,
				Labels: map[string]string{"original": "true"},
			},
		},
	)

	state, err := CaptureWorkflowCRDState(ctx, cli, testWorkflowCRDName)
	if err != nil {
		t.Fatalf("CaptureWorkflowCRDState() error = %v", err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: testWorkflowCRDName}, crd); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	crd.Labels = map[string]string{
		testManagedLabel: "dsp-e2e",
		"original":       "false",
		"patched":        "true",
	}
	if err := cli.Update(ctx, crd); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if err := RestoreWorkflowCRDState(
		ctx,
		cli,
		testWorkflowCRDName,
		state,
		testManagedLabel,
		testManagedLabelValue,
	); err != nil {
		t.Fatalf("RestoreWorkflowCRDState() error = %v", err)
	}

	restored := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: testWorkflowCRDName}, restored); err != nil {
		t.Fatalf("Get() after restore error = %v", err)
	}
	if got := restored.Labels["original"]; got != "true" {
		t.Fatalf("restored original label = %q, want %q", got, "true")
	}
	if _, exists := restored.Labels["patched"]; exists {
		t.Fatalf("patched label still present after restore")
	}
	if _, exists := restored.Labels[testManagedLabel]; exists {
		t.Fatalf("test-managed label still present after restore")
	}
}

func TestRestoreWorkflowCRDStateKeepsTestManagedStubWhenOriginallyMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cli := newFakeClient(
		t,
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: testWorkflowCRDName,
				Labels: map[string]string{
					testManagedLabel: testManagedLabelValue,
				},
			},
		},
	)

	if err := RestoreWorkflowCRDState(
		ctx,
		cli,
		testWorkflowCRDName,
		WorkflowCRDState{},
		testManagedLabel,
		testManagedLabelValue,
	); err != nil {
		t.Fatalf("RestoreWorkflowCRDState() error = %v", err)
	}

	current := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: testWorkflowCRDName}, current); err != nil {
		t.Fatalf("Get() after restore error = %v", err)
	}
	if got := current.Labels[testManagedLabel]; got != testManagedLabelValue {
		t.Fatalf("test-managed label = %q, want %q", got, testManagedLabelValue)
	}
}

func TestRestoreWorkflowCRDStateRecreatesDeletedOriginalCRD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cli := newFakeClient(
		t,
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testWorkflowCRDName,
				Labels: map[string]string{"original": "true"},
			},
		},
	)

	state, err := CaptureWorkflowCRDState(ctx, cli, testWorkflowCRDName)
	if err != nil {
		t.Fatalf("CaptureWorkflowCRDState() error = %v", err)
	}

	current := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: testWorkflowCRDName}, current); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if err := cli.Delete(ctx, current); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if err := RestoreWorkflowCRDState(
		ctx,
		cli,
		testWorkflowCRDName,
		state,
		testManagedLabel,
		testManagedLabelValue,
	); err != nil {
		t.Fatalf("RestoreWorkflowCRDState() error = %v", err)
	}

	restored := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: testWorkflowCRDName}, restored); err != nil {
		t.Fatalf("Get() after restore error = %v", err)
	}
	if got := restored.Labels["original"]; got != "true" {
		t.Fatalf("restored original label = %q, want %q", got, "true")
	}
}

func newFakeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}
