// Package gvk centralizes GroupVersionKind constants for the Workbenches module operator.
package gvk

import (
	clustergvk "github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
)

// Shared chartgen GVKs reused from the upstream operator cluster package.
var (
	Namespace                      = clustergvk.Namespace
	Deployment                     = clustergvk.Deployment
	ServiceAccount                 = clustergvk.ServiceAccount
	ConfigMap                      = clustergvk.ConfigMap
	ClusterRoleBinding             = clustergvk.ClusterRoleBinding
	RoleBinding                    = clustergvk.RoleBinding
	MutatingWebhookConfiguration   = clustergvk.MutatingWebhookConfiguration
	ValidatingWebhookConfiguration = clustergvk.ValidatingWebhookConfiguration
	CertManagerCertificate         = clustergvk.CertManagerCertificate
)

// OpenShift cluster-level types read uncached.
var (
	OpenshiftIngress = clustergvk.OpenshiftIngress
)

// Workbenches controller GVKs.
var (
	// ImageStream is watched conditionally (only if CRD exists).
	ImageStream = clustergvk.ImageStream

	// Notebook is the primary workload CRD managed by workbenches.
	Notebook = clustergvk.Notebook

	// MLflowOperator is watched to refresh mlflow-enabled param.
	MLflowOperator = clustergvk.MLflowOperator

	// GatewayConfig is used to resolve the gateway domain URL.
	GatewayConfig = clustergvk.GatewayConfig

	// KubernetesGateway is queried for the data-science-gateway hostname.
	KubernetesGateway = clustergvk.KubernetesGateway

	// OAuthClient is managed by the deployed odh-notebook-controller.
	OAuthClient = clustergvk.OAuthClient

	// HardwareProfile is fetched by the hardware profile webhook.
	HardwareProfile = clustergvk.HardwareProfile

	// LLMInferenceServiceV1Alpha1/V1Alpha2 — referenced only in the hardware profile
	// webhook for kind validation (not owned/watched by the workbenches controller).
	LLMInferenceServiceV1Alpha1 = clustergvk.LLMInferenceServiceV1Alpha1
	LLMInferenceServiceV1Alpha2 = clustergvk.LLMInferenceServiceV1Alpha2
)
