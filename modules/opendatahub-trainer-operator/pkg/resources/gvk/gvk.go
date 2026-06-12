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

// Package gvk centralizes GroupVersionKind constants for the Trainer module operator.
package gvk

import fwgvk "github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"

// Shared chartgen GVKs reused from the framework cluster package.
var (
	Namespace                      = fwgvk.Namespace
	CustomResourceDefinition       = fwgvk.CustomResourceDefinition
	Deployment                     = fwgvk.Deployment
	ServiceAccount                 = fwgvk.ServiceAccount
	ConfigMap                      = fwgvk.ConfigMap
	ClusterRoleBinding             = fwgvk.ClusterRoleBinding
	RoleBinding                    = fwgvk.RoleBinding
	MutatingWebhookConfiguration   = fwgvk.MutatingWebhookConfiguration
	ValidatingWebhookConfiguration = fwgvk.ValidatingWebhookConfiguration
	CertManagerCertificate         = fwgvk.CertManagerCertificate
)

// Trainer controller GVKs reused from the framework cluster package.
var (
	ClusterTrainingRuntime = fwgvk.ClusterTrainingRuntime
	JobSetOperatorV1       = fwgvk.JobSetOperatorV1
	JobSetv1alpha2         = fwgvk.JobSetv1alpha2
)
