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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
)

func TestBroadcastListMapper(t *testing.T) {
	g := NewWithT(t)

	claim1 := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-1", Namespace: "ns-1"},
	}
	claim2 := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-2", Namespace: "ns-2"},
	}

	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(claim1, claim2).Build()
	mapper := controller.BroadcastListMapper(cli, &infraApi.DatabaseClaimList{})

	reqs := mapper(context.Background(), nil)
	g.Expect(reqs).To(ConsistOf(
		reconcile.Request{NamespacedName: client.ObjectKeyFromObject(claim1)},
		reconcile.Request{NamespacedName: client.ObjectKeyFromObject(claim2)},
	))
}
