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

	fwgvk "github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
)

var (
	// Chartgen GVKs (shared across module Helm chart generation).
	Namespace                      = fwgvk.Namespace
	Deployment                     = fwgvk.Deployment
	ServiceAccount                 = fwgvk.ServiceAccount
	ConfigMap                      = fwgvk.ConfigMap
	ClusterRoleBinding             = fwgvk.ClusterRoleBinding
	RoleBinding                    = fwgvk.RoleBinding
	MutatingWebhookConfiguration   = fwgvk.MutatingWebhookConfiguration
	ValidatingWebhookConfiguration = fwgvk.ValidatingWebhookConfiguration
	CertManagerCertificate         = fwgvk.CertManagerCertificate

	// Controller GVKs (reconciler watches and dynamic ownership).
	MLflow = fwgvk.MLflow
	// Keep local to the MLflow module unless it becomes shared across components.
	GatewayConfig = schema.GroupVersionKind{
		Group:   "services.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "GatewayConfig",
	}

	// ConsoleLink is the OpenShift ConsoleLink GVK.
	// Keep local to the MLflow module unless it becomes shared across components.
	ConsoleLink = schema.GroupVersionKind{
		Group:   "console.openshift.io",
		Version: "v1",
		Kind:    "ConsoleLink",
	}
)
