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

package schemaclaim

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveSchema_ExplicitSchema(t *testing.T) {
	g := NewWithT(t)
	g.Expect(resolveSchema("ns", "name", "my_schema")).To(Equal("my_schema"))
}

func TestResolveSchema_Default(t *testing.T) {
	g := NewWithT(t)
	got := resolveSchema("redhat-ods-applications", "model-registry", "")
	g.Expect(got).To(Equal("redhat_ods_applications_model_registry"))
	g.Expect(len(got)).To(BeNumerically("<=", maxSchemaLen))
}

func TestResolveSchema_NonAlphanumericSanitized(t *testing.T) {
	g := NewWithT(t)
	got := resolveSchema("my-ns", "my-claim!", "")
	// Only [a-z0-9_] allowed
	for _, c := range got {
		g.Expect(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_').To(BeTrue(),
			"unexpected character %q in schema %q", string(c), got)
	}
}

func TestResolveSchema_TruncatedWithHash(t *testing.T) {
	g := NewWithT(t)
	long := strings.Repeat("a", 40)
	got := resolveSchema(long, long, "")
	g.Expect(len(got)).To(BeNumerically("<=", maxSchemaLen))
	// Two different long names must produce different schemas
	got2 := resolveSchema(long, strings.Repeat("b", 40), "")
	g.Expect(got).NotTo(Equal(got2))
}

func TestResolveSchema_Deterministic(t *testing.T) {
	g := NewWithT(t)
	a := resolveSchema("namespace", "name", "")
	b := resolveSchema("namespace", "name", "")
	g.Expect(a).To(Equal(b))
}
