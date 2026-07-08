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
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
)

func expectDatabaseProviderUnreachable(
	t *testing.T,
	env *integrationEnv,
	provider *infraApi.DatabaseProvider,
	reason string,
	messageSubstring string,
) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(env.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal(databaseprovider.ConditionReachable)),
				HaveField("Status", Equal(metav1.ConditionFalse)),
			),
		)),
	)

	current := &infraApi.DatabaseProvider{ObjectMeta: metav1.ObjectMeta{Name: provider.Name}}
	NewWithT(t).Eventually(t.Context(), k8sm.Lookup(env.Client, current)).To(Succeed())

	for _, cond := range current.Status.Conditions {
		if cond.Type != databaseprovider.ConditionReachable {
			continue
		}

		NewWithT(t).Expect(cond.Reason).To(Equal(reason))
		NewWithT(t).Expect(cond.Message).To(ContainSubstring(messageSubstring))
		return
	}

	t.Fatalf("reachable condition not found for provider %q", provider.Name)
}
