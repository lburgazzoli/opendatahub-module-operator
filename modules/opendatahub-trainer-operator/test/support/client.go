package support

import (
	"fmt"

	imagev1 "github.com/openshift/api/image/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
)

func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := admissionv1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding admissionregistration scheme: %w", err)
	}
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}
	if err := apiextensionsv1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding apiextensions scheme: %w", err)
	}
	if err := imagev1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding openshift image scheme: %w", err)
	}
	if err := promv1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding prometheus scheme: %w", err)
	}
	if err := componentsv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding trainer scheme: %w", err)
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
