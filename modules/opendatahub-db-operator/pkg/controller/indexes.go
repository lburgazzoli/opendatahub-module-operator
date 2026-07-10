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

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

const (
	// IndexEffectiveProvider is the field index key for looking up SchemaClaims
	// and DatabaseClaims by their bound provider. It is derived from
	// spec.provider.name (name-based) or status.provider (selector-based).
	IndexEffectiveProvider = "index.databaseprovider/effectiveProvider"
)

// FieldIndexer describes a single field index to register on the shared cache.
type FieldIndexer struct {
	Obj   client.Object
	Field string
	Fn    client.IndexerFunc
}

// FieldIndexers lists all field indexes the manager registers on startup.
// Exported so that tests using fake clients can register the same indexes.
var FieldIndexers = []FieldIndexer{
	{
		Obj:   &infraApi.SchemaClaim{},
		Field: IndexEffectiveProvider,
		Fn: func(obj client.Object) []string {
			claim, ok := obj.(*infraApi.SchemaClaim)
			if !ok {
				return nil
			}
			if claim.Spec.Provider.Name != "" {
				return []string{claim.Spec.Provider.Name}
			}
			if claim.Status.Provider != "" {
				return []string{claim.Status.Provider}
			}
			return nil
		},
	},
	{
		Obj:   &infraApi.DatabaseClaim{},
		Field: IndexEffectiveProvider,
		Fn: func(obj client.Object) []string {
			claim, ok := obj.(*infraApi.DatabaseClaim)
			if !ok {
				return nil
			}
			if claim.Spec.Provider.Name != "" {
				return []string{claim.Spec.Provider.Name}
			}
			if claim.Status.Provider != "" {
				return []string{claim.Status.Provider}
			}
			return nil
		},
	},
}
