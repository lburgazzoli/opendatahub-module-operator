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

// RegistrationFunc registers one or more modules with the given registry.
type RegistrationFunc func(registry Registry)

// Registry accepts module registrations and exposes orchestrator config
// so modules can derive namespace and chart paths.
type Registry interface {
	Register(m *Module)
	Namespace() string
	ChartsPath() string
}

// RegistrationFuncs collects module registration functions.
// Packages call Add() in their init() to register themselves.
var RegistrationFuncs []RegistrationFunc

// Register adds a registration function to the global list.
func Register(fn RegistrationFunc) {
	RegistrationFuncs = append(RegistrationFuncs, fn)
}
