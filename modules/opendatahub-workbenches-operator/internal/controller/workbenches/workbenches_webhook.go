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

package workbenches

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	webhookutils "github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"
)

// RegisterWebhooks registers admission webhook handlers with the manager.
// Only call this when cfg.Controller.Webhook.Enabled is true.
func (m *Module) RegisterWebhooks(mgr ctrl.Manager) error {
	m.decoder = admission.NewDecoder(mgr.GetScheme())
	m.webhookClient = mgr.GetClient()

	srv := mgr.GetWebhookServer()

	srv.Register("/platform-connection-notebook", &admission.Webhook{
		Handler:        admission.HandlerFunc(m.handleNotebookConnection),
		LogConstructor: webhookutils.NewWebhookLogConstructor("workbenches-connection-webhook"),
	})

	srv.Register("/mutate-hardware-profile", &admission.Webhook{
		Handler:        admission.HandlerFunc(m.handleHardwareProfileNotebook),
		LogConstructor: webhookutils.NewWebhookLogConstructor("workbenches-hwp-webhook"),
	})

	return nil
}
