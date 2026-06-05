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

package platformoperator

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestFlattenValues(t *testing.T) {
	t.Run("nil map", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(FlattenValues(nil)).To(BeEmpty())
	})

	t.Run("empty map", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(FlattenValues(map[string]any{})).To(BeEmpty())
	})

	t.Run("flat keys", func(t *testing.T) {
		g := NewWithT(t)

		result := FlattenValues(map[string]any{
			"key1": "val1",
			"key2": "val2",
		})

		g.Expect(result).To(Equal(map[string]string{
			"key1": "val1",
			"key2": "val2",
		}))
	})

	t.Run("one level nesting", func(t *testing.T) {
		g := NewWithT(t)

		result := FlattenValues(map[string]any{
			"distribution": map[string]any{
				"name":    "OpenDataHub",
				"version": "2.16.0",
			},
		})

		g.Expect(result).To(Equal(map[string]string{
			"distribution.name":    "OpenDataHub",
			"distribution.version": "2.16.0",
		}))
	})

	t.Run("deep nesting", func(t *testing.T) {
		g := NewWithT(t)

		result := FlattenValues(map[string]any{
			"a": map[string]any{
				"b": map[string]any{
					"c": "deep",
				},
			},
		})

		g.Expect(result).To(Equal(map[string]string{
			"a.b.c": "deep",
		}))
	})

	t.Run("mixed flat and nested", func(t *testing.T) {
		g := NewWithT(t)

		result := FlattenValues(map[string]any{
			"flat-key": "flat-val",
			"nested": map[string]any{
				"inner": "inner-val",
			},
		})

		g.Expect(result).To(Equal(map[string]string{
			"flat-key":     "flat-val",
			"nested.inner": "inner-val",
		}))
	})

	t.Run("non-string values are stringified", func(t *testing.T) {
		g := NewWithT(t)

		result := FlattenValues(map[string]any{
			"count":   42,
			"enabled": true,
			"ratio":   3.14,
		})

		g.Expect(result).To(Equal(map[string]string{
			"count":   "42",
			"enabled": "true",
			"ratio":   "3.14",
		}))
	})

	t.Run("already dot-separated keys preserved", func(t *testing.T) {
		g := NewWithT(t)

		result := FlattenValues(map[string]any{
			"distribution.name": "OpenDataHub",
		})

		g.Expect(result).To(Equal(map[string]string{
			"distribution.name": "OpenDataHub",
		}))
	})
}
