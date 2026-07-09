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

import infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"

func resolveClaimSecretName(claimName string, secretName string) string {
	if secretName != "" {
		return secretName
	}
	return claimName
}

func SecretNameForSchemaClaim(claim *infraApi.SchemaClaim) string {
	if claim == nil {
		return ""
	}
	return resolveClaimSecretName(claim.Name, claim.Spec.SecretName)
}

func SecretNameForDatabaseClaim(claim *infraApi.DatabaseClaim) string {
	if claim == nil {
		return ""
	}
	return resolveClaimSecretName(claim.Name, claim.Spec.SecretName)
}
