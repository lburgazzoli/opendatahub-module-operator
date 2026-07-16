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

//nolint:ireturn
package schemaclaim

import (
	"k8s.io/client-go/tools/events"

	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

// Option is implemented by both the Options struct literal and the named
// With* constructor functions, following the same pattern as the platform
// operator's controller packages. Pass any combination to NewModule().
type Option interface {
	applyOption(o *Options)
}

// Options configures the Module. cfg and platformRelease are always set by
// NewModule and are not user-configurable. Add fields for task-specific
// dependencies (e.g., a pgxpool in task-05, admin secret name in task-08)
// with corresponding With* constructors below.
type Options struct {
	cfg                   *moduleconfig.Config
	platformRelease       fwapi.Release
	Recorder              events.EventRecorder
	PostgresClientFactory postgres.ClientFactory
}

func (o Options) applyOption(target *Options) {
	if o.cfg != nil {
		target.cfg = o.cfg
	}
	if o.platformRelease.Name != "" || !o.platformRelease.Version.EQ(o.platformRelease.Version) {
		target.platformRelease = o.platformRelease
	}

	if o.Recorder != nil {
		target.Recorder = o.Recorder
	}
	if o.PostgresClientFactory != nil {
		target.PostgresClientFactory = o.PostgresClientFactory
	}
}

type optionFunc func(*Options)

func (fn optionFunc) applyOption(target *Options) {
	if fn == nil {
		return
	}

	fn(target)
}

func WithPostgresClientFactory(factory postgres.ClientFactory) Option {
	return optionFunc(func(target *Options) {
		if target == nil || factory == nil {
			return
		}

		target.PostgresClientFactory = factory
	})
}

func WithRecorder(recorder events.EventRecorder) Option {
	return optionFunc(func(target *Options) {
		if target == nil || recorder == nil {
			return
		}

		target.Recorder = recorder
	})
}
