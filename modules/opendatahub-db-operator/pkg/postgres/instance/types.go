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

package instance

import (
	corev1 "k8s.io/api/core/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

type Data struct {
	Namespace    string
	ProviderName string

	Service  Service
	PVC      PVC
	InitDB   InitDB
	Postgres Postgres
	Network  NetworkPolicy
	TLS      TLS
}

type Service struct {
	Name string
}

type PVC struct {
	Name             string
	Size             string
	StorageClassName string
}

type InitDB struct {
	ConfigMapName string
	Extensions    []string
}

type Postgres struct {
	Image           string
	Resources       *corev1.ResourceRequirements
	Envs            []corev1.EnvVar
	AdminSecretName string
	InstanceHash    string
}

type NetworkPolicy struct {
	AllowedNamespaces []string
}

type TLS struct {
	Enabled           bool
	UsesManagedIssuer bool
	SecretName        string
	SecretHash        string
	IssuerName        string
	IssuerRef         *infraApi.CertManagerIssuerRef
	Certificate       Certificate
}

type Certificate struct {
	Name        string
	Duration    *Duration
	RenewBefore *Duration
}

type Duration struct {
	String string
}

func Values(data Data) map[string]any {
	return map[string]any{
		"Namespace":    data.Namespace,
		"ProviderName": data.ProviderName,
		"Service":      data.Service,
		"PVC":          data.PVC,
		"InitDB":       data.InitDB,
		"Postgres":     data.Postgres,
		"Network":      data.Network,
		"TLS":          data.TLS,
	}
}
