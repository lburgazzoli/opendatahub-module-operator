package support

import (
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	infrastructurev1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	servicesv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/services/v1alpha1"
)

// NewScheme registers the generic types plus this module's own CRD schemes
// (SchemaClaim/DatabaseClaim/DatabaseProvider, DatabaseService -- task-02).
func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}
	if err := apiextensionsv1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding apiextensions scheme: %w", err)
	}
	if err := infrastructurev1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding infrastructure scheme: %w", err)
	}
	if err := servicesv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding services scheme: %w", err)
	}

	return s, nil
}

func NewClient() (client.Client, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}

	s, err := NewScheme()
	if err != nil {
		return nil, err
	}

	cli, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	return cli, nil
}
