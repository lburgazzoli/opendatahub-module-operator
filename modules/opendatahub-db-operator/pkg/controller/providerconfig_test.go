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

package controller_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func providerConfigScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = infraApi.AddToScheme(scheme)
	return scheme
}

func TestLoadProviderConfig_EmbeddedUsesGeneratedAdminSecret(t *testing.T) {
	g := NewWithT(t)

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sample-embedded",
		},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				Namespace: "opendatahub-db",
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.EmbeddedAdminSecretName(provider.Name),
			Namespace: "opendatahub-db",
		},
		Data: map[string][]byte{
			controller.EmbeddedAdminSecretUserKey:     []byte("postgres"),
			controller.EmbeddedAdminSecretPasswordKey: []byte("postgres"),
			controller.EmbeddedAdminSecretDBKey:       []byte("postgres"),
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(providerConfigScheme()).
		WithObjects(secret).
		Build()

	cfg, err := controller.LoadProviderConfig(
		context.Background(),
		cli,
		provider,
		"odh-db-operator-system",
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).To(Equal(postgres.Config{
		Host:     "sample-embedded.opendatahub-db.svc.cluster.local",
		Port:     postgres.DefaultPort,
		User:     "postgres",
		Password: "postgres",
		DBName:   "postgres",
	}))
}
