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

// Package controller provides cross-cutting helpers shared by all controllers
// in this module.
package controller

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
)

// MigrateFn is called by UpgradeIfNeeded when the running platform version is
// newer than what the object last reconciled on. It runs idempotent migrations
// for a single version advance. The function receives the current reconcile
// request and returns an error if the migration fails.
type MigrateFn func(ctx context.Context, rr *odhtypes.ReconciliationRequest) error

// UpgradeIfNeeded returns an action.Fn that checks whether the running platform
// version (rr.Release.Version) is newer than the version recorded in the
// object's status.releases[name=platform]. If it is, it calls each provided
// MigrateFn in order, then records the new version. If no fns are provided it
// is a no-op (used in controllers that have no migration logic yet).
//
// This follows the same platform-version-gating pattern every other module
// operator uses (docs/plan.md §6), centralised here so individual controllers
// don't each repeat the semver parsing / release lookup boilerplate.
func UpgradeIfNeeded(fns ...MigrateFn) actions.Fn {
	return func(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
		withReleases, ok := rr.Instance.(common.WithReleases)
		if !ok {
			return fmt.Errorf("resource instance does not implement WithReleases")
		}

		var prevVersion semver.Version
		if prev := lookupPlatformRelease(withReleases.GetReleaseStatus()); prev.Version != "" {
			var err error
			if prevVersion, err = semver.ParseTolerant(prev.Version); err != nil {
				return fmt.Errorf("parsing previous platform version %q: %w", prev.Version, err)
			}
		}

		if !rr.Release.Version.GT(prevVersion) {
			return nil
		}

		for _, fn := range fns {
			if err := fn(ctx, rr); err != nil {
				return err
			}
		}

		return nil
	}
}

func lookupPlatformRelease(status *common.ComponentReleaseStatus) common.ComponentRelease {
	if status == nil {
		return common.ComponentRelease{}
	}
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r
		}
	}
	return common.ComponentRelease{}
}
