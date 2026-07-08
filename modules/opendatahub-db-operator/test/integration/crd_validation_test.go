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

package integration

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

// TestCRDValidation is task-02's integration test (docs/task-02.md step 9):
// every schema/CEL rule added in task-02 must be enforced by kube-apiserver
// itself against the CRDs installed on the connected cluster -- none of this
// can be verified with a fake client.
func TestCRDValidation(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	cli, err := support.NewClient()
	g.Expect(err).NotTo(HaveOccurred())
	ns := support.IntegrationTestNamespace()
	g.Expect(support.EnsureNamespace(ctx, cli, ns)).To(Succeed())

	t.Run("DatabaseProvider", func(t *testing.T) {
		externalSpec := infraApi.ExternalProviderSpec{
			ConnectionSecretRef: corev1.SecretReference{Name: "admin-secret", Namespace: ns},
		}
		embeddedSpec := infraApi.EmbeddedProviderSpec{
			Storage: infraApi.StorageSpec{Size: resource.MustParse("1Gi")},
		}

		cases := []struct {
			name string
			spec infraApi.DatabaseProviderSpec
		}{
			{
				"both-set",
				infraApi.DatabaseProviderSpec{
					Type:     infraApi.ProviderTypeExternal,
					External: &externalSpec,
					Embedded: &embeddedSpec,
				},
			},
			{"neither-set", infraApi.DatabaseProviderSpec{Type: infraApi.ProviderTypeExternal}},
			{
				"type-mismatch-external",
				infraApi.DatabaseProviderSpec{
					Type:     infraApi.ProviderTypeExternal,
					Embedded: &embeddedSpec,
				},
			},
			{
				"type-mismatch-embedded",
				infraApi.DatabaseProviderSpec{
					Type:     infraApi.ProviderTypeEmbedded,
					External: &externalSpec,
				},
			},
		}
		for _, tc := range cases {
			t.Run("rejects-"+tc.name, func(t *testing.T) {
				g := NewWithT(t)
				obj := &infraApi.DatabaseProvider{
					ObjectMeta: metav1.ObjectMeta{Name: "dbp-" + tc.name},
					Spec:       tc.spec,
				}
				g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
			})
		}

		t.Run("accepts-valid-external", func(t *testing.T) {
			g := NewWithT(t)
			obj := &infraApi.DatabaseProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "dbp-valid-external"},
				Spec:       infraApi.DatabaseProviderSpec{Type: infraApi.ProviderTypeExternal, External: &externalSpec},
			}
			g.Expect(cli.Create(ctx, obj)).To(Succeed())
			t.Cleanup(func() { _ = cli.Delete(ctx, obj) })
		})

		t.Run("accepts-valid-embedded", func(t *testing.T) {
			g := NewWithT(t)
			obj := &infraApi.DatabaseProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "dbp-valid-embedded"},
				Spec:       infraApi.DatabaseProviderSpec{Type: infraApi.ProviderTypeEmbedded, Embedded: &embeddedSpec},
			}
			g.Expect(cli.Create(ctx, obj)).To(Succeed())
			t.Cleanup(func() { _ = cli.Delete(ctx, obj) })
		})

		t.Run("rejects-invalid-extension-name", func(t *testing.T) {
			g := NewWithT(t)
			bad := embeddedSpec
			bad.Extensions = []string{"Not-Valid!"}
			obj := &infraApi.DatabaseProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "dbp-bad-extension"},
				Spec:       infraApi.DatabaseProviderSpec{Type: infraApi.ProviderTypeEmbedded, Embedded: &bad},
			}
			g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
		})
	})
}
