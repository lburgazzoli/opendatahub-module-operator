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

// Package maps provides map helpers that complement the stdlib maps package.
package maps

// Set stores key→value in m, allocating m if it is nil, and returns the map.
// It is the missing complement to the built-in delete: a safe single-entry
// write that handles the nil-map case without a separate initialisation step.
//
//	rr.Extensions = maps.Set(rr.Extensions, "key", value)
func Set[K comparable, V any](m map[K]V, key K, value V) map[K]V {
	if m == nil {
		m = make(map[K]V)
	}
	m[key] = value
	return m
}
