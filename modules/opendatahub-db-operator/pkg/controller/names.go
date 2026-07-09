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
	"fmt"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

func SecretNameForSchemaClaim(claim *infraApi.SchemaClaim) string {
	switch {
	case claim == nil:
		return ""
	case claim.Spec.SecretName != "":
		return claim.Spec.SecretName
	default:
		return claim.Name
	}
}

func SecretNameForDatabaseClaim(claim *infraApi.DatabaseClaim) string {
	switch {
	case claim == nil:
		return ""
	case claim.Spec.SecretName != "":
		return claim.Spec.SecretName
	default:
		return claim.Name
	}
}

func EmbeddedAdminSecretName(providerName string) string {
	return providerName + "-admin"
}

func EmbeddedServiceName(providerName string) string {
	return providerName
}

func EmbeddedPVCName(providerName string) string {
	return providerName
}

func EmbeddedInitDBConfigMapName(providerName string) string {
	return providerName + "-initdb"
}

func EmbeddedNamespace(provider *infraApi.DatabaseProvider, operatorNamespace string) string {
	if provider != nil && provider.Spec.Embedded != nil && provider.Spec.Embedded.Namespace != "" {
		return provider.Spec.Embedded.Namespace
	}
	return operatorNamespace
}

func EmbeddedServiceHost(provider *infraApi.DatabaseProvider, operatorNamespace string) string {
	if provider == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s.%s.svc",
		EmbeddedServiceName(provider.Name),
		EmbeddedNamespace(provider, operatorNamespace),
	)
}
