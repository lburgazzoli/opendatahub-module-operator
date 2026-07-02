// Package gvk centralizes GroupVersionKind constants for the Workbenches module operator.
package gvk

import (
	oauthv1 "github.com/openshift/api/oauth/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

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

// OpenShift cluster-level types read uncached.
var (
	OpenshiftIngress = fwgvk.OpenshiftIngress
)

// Workbenches controller GVKs.
var (
	// ImageStream is watched conditionally (only if CRD exists).
	ImageStream = fwgvk.ImageStream

	// Notebook is the primary workload CRD managed by workbenches.
	Notebook = fwgvk.Notebook

	// MLflowOperator is watched to refresh mlflow-enabled param.
	// Keep local to the Workbenches module unless it becomes shared across components.
	MLflowOperator = schema.GroupVersionKind{
		Group:   "components.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "MLflowOperator",
	}

	// GatewayConfig is used to resolve the gateway domain URL.
	// Keep local to the Workbenches module unless it becomes shared across components.
	GatewayConfig = schema.GroupVersionKind{
		Group:   "services.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "GatewayConfig",
	}

	// KubernetesGateway is queried for the data-science-gateway hostname.
	// Keep local to the Workbenches module unless it becomes shared across components.
	KubernetesGateway = schema.GroupVersionKind{
		Group:   gwapiv1.GroupVersion.Group,
		Version: gwapiv1.GroupVersion.Version,
		Kind:    "Gateway",
	}

	// OAuthClient is managed by the deployed odh-notebook-controller.
	// Keep local to the Workbenches module unless it becomes shared across components.
	OAuthClient = schema.GroupVersionKind{
		Group:   oauthv1.GroupVersion.Group,
		Version: oauthv1.GroupVersion.Version,
		Kind:    "OAuthClient",
	}

	// HardwareProfile is fetched by the hardware profile webhook.
	// Keep local to the Workbenches module unless it becomes shared across components.
	HardwareProfile = schema.GroupVersionKind{
		Group:   "infrastructure.opendatahub.io",
		Version: "v1",
		Kind:    "HardwareProfile",
	}

	// OdhDashboardConfig drives notebook controller and workbench UI behavior.
	OdhDashboardConfig = fwgvk.OdhDashboardConfig

	// DashboardAcceleratorProfile is referenced by webhook validation.
	DashboardAcceleratorProfile = fwgvk.DashboardAcceleratorProfile

	// LLMInferenceServiceV1Alpha1/V1Alpha2 — referenced only in the hardware profile
	// webhook for kind validation (not owned/watched by the workbenches controller).
	LLMInferenceServiceV1Alpha1 = fwgvk.LLMInferenceServiceV1Alpha1
	LLMInferenceServiceV1Alpha2 = fwgvk.LLMInferenceServiceV1Alpha2
)
