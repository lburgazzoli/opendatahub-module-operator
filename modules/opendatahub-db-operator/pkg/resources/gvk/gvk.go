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
	CertManagerCertificate         = fwgvk.CertManagerCertificate
	Secret                         = fwgvk.Secret
)

// Module-specific GVKs (SchemaClaim, DatabaseClaim, DatabaseProvider, StatefulSet,
// PersistentVolumeClaim, Service, NetworkPolicy) are added in task-02/task-08 once
// those types and the Embedded provider's owned resources exist (docs/plan.md §5, §7).
