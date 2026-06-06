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

package module

import "sigs.k8s.io/controller-runtime/pkg/event"

// Orchestration provides the module-facing view of the platform orchestrator's state.
// Implemented by the Orchestrator in the platform controller package; consumed by
// the PlatformOperator controller without importing it.
type Orchestration interface {
	ShouldReconcileModule(m *Module) bool
	ModuleByName(name string) *Module
	StateChanges() <-chan event.GenericEvent
}
