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
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = infraApi.AddToScheme(s)
	return s
}

func makeProvider(name string, labels map[string]string, annotations map[string]string) infraApi.DatabaseProvider {
	return infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{Name: "secret"},
			},
		},
	}
}

func TestResolveByName_Found(t *testing.T) {
	g := NewWithT(t)
	p := makeProvider("my-provider", nil, nil)
	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(&p).Build()
	ref := infraApi.ProviderRef{Name: "my-provider"}
	got, err := controller.Resolve(context.Background(), cli, ref)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Name).To(Equal("my-provider"))
}

func TestResolveByName_NotFound(t *testing.T) {
	g := NewWithT(t)
	cli := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	ref := infraApi.ProviderRef{Name: "missing"}
	_, err := controller.Resolve(context.Background(), cli, ref)
	g.Expect(err).To(HaveOccurred())

	var notFound controller.ErrNotFound
	g.Expect(errors.As(err, &notFound)).To(BeTrue())
}

func TestResolveBySelector_SingleMatch(t *testing.T) {
	g := NewWithT(t)
	p := makeProvider("p1", map[string]string{"cap": "pgvector"}, nil)
	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(&p).Build()
	ref := infraApi.ProviderRef{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"cap": "pgvector"}},
	}
	got, err := controller.Resolve(context.Background(), cli, ref)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Name).To(Equal("p1"))
}

func TestResolveBySelector_ZeroMatches(t *testing.T) {
	g := NewWithT(t)
	cli := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	ref := infraApi.ProviderRef{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"cap": "nonexistent"}},
	}
	_, err := controller.Resolve(context.Background(), cli, ref)
	g.Expect(err).To(HaveOccurred())
	var notFound controller.ErrNotFound
	g.Expect(errors.As(err, &notFound)).To(BeTrue())
}

func TestResolveBySelector_MultiMatch_PriorityTieBreak(t *testing.T) {
	g := NewWithT(t)

	p1 := makeProvider("z-low-priority", map[string]string{"cap": "vec"}, map[string]string{
		controller.AnnotationSelectionPriority: "5",
	})
	p2 := makeProvider("a-high-priority", map[string]string{"cap": "vec"}, map[string]string{
		controller.AnnotationSelectionPriority: "10",
	})
	p3 := makeProvider("b-higher-priority", map[string]string{"cap": "vec"}, map[string]string{
		controller.AnnotationSelectionPriority: "10",
	})

	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(&p1, &p2, &p3).Build()
	ref := infraApi.ProviderRef{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"cap": "vec"}},
	}
	got, err := controller.Resolve(context.Background(), cli, ref)
	g.Expect(err).NotTo(HaveOccurred())
	// a-high-priority and b-higher-priority both have priority 10; alphabetically "a" < "b"
	g.Expect(got.Name).To(Equal("a-high-priority"))
}

func TestResolveBySelector_MultiMatch_AlphabeticTieBreak(t *testing.T) {
	g := NewWithT(t)

	p1 := makeProvider("bravo", map[string]string{"cap": "vec"}, nil)
	p2 := makeProvider("alpha", map[string]string{"cap": "vec"}, nil)
	p3 := makeProvider("charlie", map[string]string{"cap": "vec"}, nil)

	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(&p1, &p2, &p3).Build()
	ref := infraApi.ProviderRef{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"cap": "vec"}},
	}
	got, err := controller.Resolve(context.Background(), cli, ref)
	g.Expect(err).NotTo(HaveOccurred())
	// All have no priority annotation (= 0); alphabetically "alpha" < "bravo" < "charlie"
	g.Expect(got.Name).To(Equal("alpha"))
}

func TestResolveDefault_Found(t *testing.T) {
	g := NewWithT(t)
	p := makeProvider("default-provider", nil, map[string]string{
		controller.AnnotationIsDefault: "true",
	})
	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(&p).Build()
	got, err := controller.Resolve(context.Background(), cli, infraApi.ProviderRef{})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Name).To(Equal("default-provider"))
}

func TestResolveDefault_NoDefault(t *testing.T) {
	g := NewWithT(t)
	p := makeProvider("not-default", nil, nil)
	cli := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(&p).Build()
	_, err := controller.Resolve(context.Background(), cli, infraApi.ProviderRef{})
	g.Expect(err).To(HaveOccurred())
	var notFound controller.ErrNotFound
	g.Expect(errors.As(err, &notFound)).To(BeTrue())
}

func TestResolveDefault_NoneAtAll(t *testing.T) {
	g := NewWithT(t)
	cli := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	_, err := controller.Resolve(context.Background(), cli, infraApi.ProviderRef{})
	g.Expect(err).To(HaveOccurred())
}
