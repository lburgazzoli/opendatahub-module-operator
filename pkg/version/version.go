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

package version

// Build metadata, overridden at build time via ldflags:
//
//	go build -ldflags "\
//	  -X github.com/lburgazzoli/opendatahub-module-operator/pkg/version.Version=1.0.0 \
//	  -X github.com/lburgazzoli/opendatahub-module-operator/pkg/version.Commit=abc1234 \
//	  -X github.com/lburgazzoli/opendatahub-module-operator/pkg/version.Branch=main \
//	  -X github.com/lburgazzoli/opendatahub-module-operator/pkg/version.Repo=github.com/lburgazzoli/opendatahub-module-operator"
var (
	Version = "0.0.0-dev"
	Commit  = "unknown"
	Branch  = "unknown"
	Repo    = "unknown"
)
