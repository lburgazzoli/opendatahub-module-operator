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

package datasciencepipelines

import (
	"context"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/resources/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/deploy"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/gc"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/render/kustomize"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/status/deployments"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/status/releases"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/handlers"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/predicates"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/predicates/component"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/reconciler"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Module CRD.
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=datasciencepipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=datasciencepipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=datasciencepipelines/finalizers,verbs=update
// Baseline controller RBAC.
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch;update
// +kubebuilder:rbac:urls=/metrics,verbs=get
// Resources owned by the reconciler.
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;services;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// Operand RBAC required to apply the DSP Argo and manager roles.
// +kubebuilder:rbac:groups="",resources=configmaps;events;namespaces;persistentvolumeclaims;persistentvolumeclaims/finalizers;persistentvolumes;pods;pods/exec;pods/log;replicasets;secrets;serviceaccounts;services,verbs=*
// +kubebuilder:rbac:groups="",resources=deployments;deployments/finalizers,verbs=*
// +kubebuilder:rbac:groups=*,resources=deployments;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps;extensions,resources=deployments;deployments/finalizers;replicasets,verbs=*
// +kubebuilder:rbac:groups=argoproj.io,resources=clusterworkflowtemplates;clusterworkflowtemplates/finalizers;cronworkflows;cronworkflows/finalizers;workflowartifactgctasks;workflowartifactgctasks/finalizers;workfloweventbindings;workfloweventbindings/finalizers;workflows;workflows/finalizers;workflowtaskresults;workflowtaskresults/finalizers;workflowtasksets;workflowtasksets/finalizers;workflowtemplates;workflowtemplates/finalizers,verbs=*
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=*
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=*
// +kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications;datasciencepipelinesapplications/api,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreamtags,verbs=get
// +kubebuilder:rbac:groups=kubeflow.org,resources=*,verbs=*
// +kubebuilder:rbac:groups=machinelearning.seldon.io,resources=seldondeployments,verbs=*
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pipelines.kubeflow.org,resources=pipelines;pipelines/finalizers;pipelineversions;pipelineversions/finalizers;pipelineversions/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;create;delete
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters;rayjobs;rayservices,verbs=get;list;create;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;create;patch;delete
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;create;delete
// +kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers;appwrappers/finalizers;appwrappers/status,verbs=get;list;watch;create;update;patch;delete;deletecollection

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
	rel common.Release,
) error {
	m, err := NewModule(cfg)
	if err != nil {
		return err
	}

	r, err := reconciler.ReconcilerFor(mgr, &componentApi.DataSciencePipelines{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&promv1.ServiceMonitor{}).
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		OwnsGVK(gvk.SecurityContextConstraints).
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(handlers.ToNamed(componentApi.DataSciencePipelinesInstanceName)),
			reconciler.WithPredicates(component.ForLabel(labels.ODH.Component(LegacyComponentName), labels.True)),
		).
		WithAction(checkPreConditions).
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(argoWorkflowsControllersOptions).
		WithAction(releases.NewAction()).
		WithAction(kustomize.NewAction(
			kustomize.WithLabel(labels.ODH.Component(LegacyComponentName), labels.True),
			kustomize.WithLabel(labels.K8SCommon.PartOf, LegacyComponentName),
		)).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
		)).
		WithAction(deployments.NewAction()).
		WithAction(m.reportStatus).
		WithAction(gc.NewAction(
			gc.InNamespace(cfg.ApplicationsNamespace),
		)).
		WithConditions(
			status.ConditionArgoWorkflowAvailable,
			status.ConditionDeploymentsAvailable,
		).
		Build(ctx)
	if err != nil {
		return err
	}

	r.Release = rel

	return nil
}
