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

package mymodule

import (
	"context"
	"encoding/json"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
	webhookutils "github.com/opendatahub-io/opendatahub-operator/v2/pkg/webhook"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
)

// ModuleHandlerFunc is a webhook handler that receives the singleton MyModule
// CR as an additional argument, loaded by wrapWithModuleCR before dispatch.
type ModuleHandlerFunc func(context.Context, admission.Request, *componentApi.MyModule) admission.Response

// RegisterWebhooks registers admission webhook handlers with the manager.
// The caller is responsible for checking cfg.WebhooksEnabled before calling
// this method — when webhooks are disabled, skip this call entirely so the
// manager does not start the webhook server or require TLS certs.
func (m *Module) RegisterWebhooks(mgr ctrl.Manager) error {
	m.decoder = admission.NewDecoder(mgr.GetScheme())
	m.apiReader = mgr.GetAPIReader()

	srv := mgr.GetWebhookServer()

	srv.Register("/mymodule-mutate-deploy", &admission.Webhook{
		Handler:        m.wrapWithModuleCR(m.labelDeployment),
		LogConstructor: webhookutils.NewWebhookLogConstructor("mymodule-deploy-mutator"),
	})

	return nil
}

// wrapWithModuleCR wraps a handler that needs the singleton MyModule CR.
// It loads the CR via the non-cached APIReader and passes it as an
// explicit argument. If the CR does not exist, the request is allowed
// (nothing to enforce).
func (m *Module) wrapWithModuleCR(fn ModuleHandlerFunc) admission.HandlerFunc {
	return func(ctx context.Context, req admission.Request) admission.Response {
		mod := &componentApi.MyModule{}
		key := client.ObjectKey{Name: componentApi.MyModuleInstanceName}

		if err := m.apiReader.Get(ctx, key, mod); err != nil {
			if k8serr.IsNotFound(err) {
				return admission.Allowed("")
			}

			return admission.Errored(http.StatusInternalServerError, err)
		}

		return fn(ctx, req, mod)
	}
}

// labelDeployment is a mutating webhook handler that injects module version
// and platform labels from the MyModule CR status onto Deployments that
// carry the platform.opendatahub.io/part-of label.
//
// This demonstrates the pattern of using the singleton CR's status to
// enrich workload resources at admission time.
//
// +kubebuilder:webhook:path=/mymodule-mutate-deploy,mutating=true,failurePolicy=ignore,sideEffects=None,groups=apps,resources=deployments,verbs=create;update,versions=v1,name=mymodule-deploy-mutator.opendatahub.io,admissionReviewVersions=v1

func (m *Module) labelDeployment(
	_ context.Context,
	req admission.Request,
	mod *componentApi.MyModule,
) admission.Response {
	if mod.Status.Module.Version.IsZero() {
		return admission.Allowed("")
	}

	deploy := &appsv1.Deployment{}
	if err := m.decoder.Decode(req, deploy); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	resources.SetLabel(deploy, "mymodule.opendatahub.io/version", mod.Status.Module.Version.String())
	resources.SetLabel(deploy, "mymodule.opendatahub.io/platform", mod.Status.Module.Platform.Name)

	mutated, err := json.Marshal(deploy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, mutated)
}
