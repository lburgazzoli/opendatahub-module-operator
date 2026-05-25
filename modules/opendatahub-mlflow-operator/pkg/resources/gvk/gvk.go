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

// Package gvk centralizes GroupVersionKind constants for the MLflow module operator.
// Shared values are re-exported from the upstream operator gvk package; module-specific
// GVKs are defined locally so callers import only this package.
package gvk

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	odhgvk "github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
)

var (
	// Chartgen GVKs (shared across module Helm chart generation).
	Namespace                      = odhgvk.Namespace
	Deployment                     = odhgvk.Deployment
	ServiceAccount                 = odhgvk.ServiceAccount
	ConfigMap                      = odhgvk.ConfigMap
	ClusterRoleBinding             = odhgvk.ClusterRoleBinding
	RoleBinding                    = odhgvk.RoleBinding
	MutatingWebhookConfiguration   = odhgvk.MutatingWebhookConfiguration
	ValidatingWebhookConfiguration = odhgvk.ValidatingWebhookConfiguration
	CertManagerCertificate         = odhgvk.CertManagerCertificate

	// Controller GVKs (reconciler watches and dynamic ownership).
	MLflow        = odhgvk.MLflow
	GatewayConfig = odhgvk.GatewayConfig

	// ConsoleLink is the OpenShift ConsoleLink GVK (no upstream gvk.* constant).
	ConsoleLink = schema.GroupVersionKind{
		Group:   "console.openshift.io",
		Version: "v1",
		Kind:    "ConsoleLink",
	}
)
