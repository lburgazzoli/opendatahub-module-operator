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

package predicates

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

func TestLogAllEvents(t *testing.T) {
	g := NewWithT(t)
	predicate := LogAllEvents("test")
	obj := &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
	}

	g.Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeTrue())
	g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj})).To(BeTrue())
	g.Expect(predicate.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
	g.Expect(predicate.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
}

func TestObjectRef(t *testing.T) {
	t.Run("cluster scoped objects use name", func(t *testing.T) {
		g := NewWithT(t)
		obj := &configApi.PlatformOperator{
			ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
		}

		g.Expect(objectRef(obj)).To(Equal("alpha"))
	})

	t.Run("namespaced objects use namespace slash name", func(t *testing.T) {
		g := NewWithT(t)
		obj := &configApi.PlatformOperator{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "opendatahub",
				Name:      "alpha",
			},
		}

		g.Expect(objectRef(obj)).To(Equal("opendatahub/alpha"))
	})
}

func TestNamed(t *testing.T) {
	g := NewWithT(t)
	p := Named(types.NamespacedName{
		Namespace: "opendatahub",
		Name:      "opendatahub-admin",
	})

	matching := &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "opendatahub",
			Name:      "opendatahub-admin",
		},
	}
	wrongNamespace := &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "other",
			Name:      "opendatahub-admin",
		},
	}
	wrongName := &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "opendatahub",
			Name:      "something-else",
		},
	}

	g.Expect(p.Create(event.CreateEvent{Object: matching})).To(BeTrue())
	g.Expect(p.Create(event.CreateEvent{Object: wrongNamespace})).To(BeFalse())
	g.Expect(p.Create(event.CreateEvent{Object: wrongName})).To(BeFalse())
}
