/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package manager

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	configv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(configv1alpha1.AddToScheme(scheme))

	return scheme
}

func buildCacheNamespaces(registry *module.Registry) map[string]cache.Config {
	namespaces := make(map[string]cache.Config)

	partOfRequirement, err := k8slabels.NewRequirement(
		odhLabels.PlatformPartOf,
		selection.Exists,
		nil,
	)
	utilruntime.Must(err)

	partOfSelector := k8slabels.NewSelector().Add(*partOfRequirement)

	for _, ns := range registry.CacheNamespaces() {
		namespaces[ns] = cache.Config{
			LabelSelector: partOfSelector,
		}
	}

	namespaces[cache.AllNamespaces] = cache.Config{
		LabelSelector: partOfSelector,
	}

	return namespaces
}
