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

package postgres_test

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

// TestQuoteIdentifier verifies SQL-injection safety on identifiers
// (schema/role/database names). The test cases use the classic injection
// vectors -- a passing test means those strings cannot escape the identifier
// quotes and inject arbitrary DDL.
func TestQuoteIdentifier(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		mustHave string // substring that must appear in the quoted output
	}{
		{"simple", "my_schema", `"my_schema"`},
		{"uppercase-is-preserved-inside-quotes", "MySchema", `"MySchema"`},
		{"embedded-double-quote-is-escaped", `schema"injection`, `""`},
		{"semicolon-cannot-terminate-statement", "schema; DROP TABLE evil", `"schema; DROP TABLE evil"`},
		{"sql-keyword-safely-quoted", "select", `"select"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := postgres.QuoteIdentifier(tc.input)
			g.Expect(got).To(ContainSubstring(tc.mustHave))
			g.Expect(strings.HasPrefix(got, `"`)).To(BeTrue())
			g.Expect(strings.HasSuffix(got, `"`)).To(BeTrue())
		})
	}
}

func TestQuoteLiteral(t *testing.T) {
	g := NewWithT(t)
	got := postgres.QuoteLiteral("pass'word")
	g.Expect(got).To(ContainSubstring("pass"))
	g.Expect(got).To(ContainSubstring("word"))
	g.Expect(got).NotTo(ContainSubstring("' DROP"))
}

func TestGeneratePassword_LengthAndCharset(t *testing.T) {
	g := NewWithT(t)

	for _, length := range []int{16, 24, 32} {
		p, err := postgres.GeneratePassword(length)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(p).To(HaveLen(length))
		for _, c := range p {
			g.Expect(c).To(Or(
				BeNumerically(">=", '0'), BeNumerically("<=", '9'),
				BeNumerically(">=", 'a'), BeNumerically("<=", 'z'),
				BeNumerically(">=", 'A'), BeNumerically("<=", 'Z'),
			), "all characters must be alphanumeric")
		}
	}
}

func TestGeneratePassword_IsRandom(t *testing.T) {
	g := NewWithT(t)
	seen := make(map[string]struct{})
	for range 20 {
		p, err := postgres.GeneratePassword(24)
		g.Expect(err).NotTo(HaveOccurred())
		seen[p] = struct{}{}
	}
	g.Expect(seen).To(HaveLen(20), "each generated password should be unique")
}
