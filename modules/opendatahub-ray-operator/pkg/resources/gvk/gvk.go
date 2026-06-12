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

// Package gvk centralizes GroupVersionKind constants for the Ray module operator.
package gvk

import (
	securityv1 "github.com/openshift/api/security/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
)

// Ray controller GVKs reused from the upstream operator cluster package.
var (
	// Keep local to the Ray module unless it becomes shared across components.
	SecurityContextConstraints = schema.GroupVersionKind{
		Group:   securityv1.SchemeGroupVersion.Group,
		Version: securityv1.SchemeGroupVersion.Version,
		Kind:    "SecurityContextConstraints",
	}
	// Keep local to the Ray module unless it becomes shared across components.
	CodeFlare = schema.GroupVersionKind{
		Group:   "components.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "CodeFlare",
	}
)
