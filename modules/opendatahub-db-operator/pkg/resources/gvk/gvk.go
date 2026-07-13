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

// Package gvk centralizes GroupVersionKind constants for the Database System Service
// module operator. This is the only place module code may import the platform's
// cluster-scoped GVK package directly (docs/plan.md §3).
package gvk

import (
	fwgvk "github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"k8s.io/apimachinery/pkg/runtime/schema"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

// Shared chartgen GVKs reused from the upstream operator cluster package.
var (
	Namespace                      = fwgvk.Namespace
	Deployment                     = fwgvk.Deployment
	ServiceAccount                 = fwgvk.ServiceAccount
	ConfigMap                      = fwgvk.ConfigMap
	ClusterRoleBinding             = fwgvk.ClusterRoleBinding
	RoleBinding                    = fwgvk.RoleBinding
	MutatingWebhookConfiguration   = fwgvk.MutatingWebhookConfiguration
	ValidatingWebhookConfiguration = fwgvk.ValidatingWebhookConfiguration
	CertManagerIssuer              = fwgvk.CertManagerIssuer
	CertManagerCertificate         = fwgvk.CertManagerCertificate
	Secret                         = fwgvk.Secret
)

// Module-specific GVKs used across controllers, tests, and chart generation.
var (
	SchemaClaim     = infraApi.SchemeGroupVersion.WithKind(infraApi.SchemaClaimKind)
	SchemaClaimList = schema.GroupVersionKind{
		Group:   SchemaClaim.Group,
		Version: SchemaClaim.Version,
		Kind:    infraApi.SchemaClaimKind,
	}

	DatabaseClaim     = infraApi.SchemeGroupVersion.WithKind(infraApi.DatabaseClaimKind)
	DatabaseClaimList = schema.GroupVersionKind{
		Group:   DatabaseClaim.Group,
		Version: DatabaseClaim.Version,
		Kind:    infraApi.DatabaseClaimKind,
	}

	DatabaseProvider     = infraApi.SchemeGroupVersion.WithKind(infraApi.DatabaseProviderKind)
	DatabaseProviderList = schema.GroupVersionKind{
		Group:   DatabaseProvider.Group,
		Version: DatabaseProvider.Version,
		Kind:    infraApi.DatabaseProviderKind,
	}
)
