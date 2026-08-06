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

func InternalAdminSecretName(providerName string) string {
	return providerName + "-admin"
}

func InternalServiceName(providerName string) string {
	return providerName
}

func InternalPVCName(providerName string) string {
	return providerName
}

func InternalInitDBConfigMapName(providerName string) string {
	return providerName + "-initdb"
}

func InternalTLSIssuerName(providerName string) string {
	return providerName + "-tls"
}

func InternalTLSCertificateName(providerName string) string {
	return providerName + "-tls"
}

func InternalTLSSecretName(providerName string) string {
	return providerName + "-tls"
}

func InternalNamespace(provider *infraApi.DatabaseProvider, operatorNamespace string) string {
	if provider != nil && provider.Spec.Internal != nil && provider.Spec.Internal.Namespace != "" {
		return provider.Spec.Internal.Namespace
	}
	return operatorNamespace
}

func InternalServiceHost(provider *infraApi.DatabaseProvider, operatorNamespace string) string {
	if provider == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s.%s.svc",
		InternalServiceName(provider.Name),
		InternalNamespace(provider, operatorNamespace),
	)
}
