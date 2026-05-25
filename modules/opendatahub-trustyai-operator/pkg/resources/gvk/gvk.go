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

// Package gvk centralizes GroupVersionKind constants for the TrustyAI module operator.
package gvk

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	clustergvk "github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
)

var (
	// Chartgen GVKs shared by all module chart generators.
	Namespace                      = clustergvk.Namespace
	Deployment                     = clustergvk.Deployment
	ServiceAccount                 = clustergvk.ServiceAccount
	ConfigMap                      = clustergvk.ConfigMap
	ClusterRoleBinding             = clustergvk.ClusterRoleBinding
	RoleBinding                    = clustergvk.RoleBinding
	MutatingWebhookConfiguration   = clustergvk.MutatingWebhookConfiguration
	ValidatingWebhookConfiguration = clustergvk.ValidatingWebhookConfiguration
	CertManagerCertificate         = clustergvk.CertManagerCertificate

	// TrustyAI dependency GVKs.
	InferenceServices = clustergvk.InferenceServices
	Kserve            = schema.GroupVersionKind{
		Group:   "components.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "Kserve",
	}
)
