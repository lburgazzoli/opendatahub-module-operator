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

package support

import (
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Platform() *configApi.Platform {
	return &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}
}

func PlatformOwner() *configApi.Platform {
	return &configApi.Platform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: configApi.GroupVersion.String(),
			Kind:       configApi.PlatformKind,
		},
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}
}

func PlatformOperator(name string) *configApi.PlatformOperator {
	return &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func PlatformOperatorOwner(name string) *configApi.PlatformOperator {
	return &configApi.PlatformOperator{
		TypeMeta: metav1.TypeMeta{
			APIVersion: configApi.GroupVersion.String(),
			Kind:       configApi.PlatformOperatorKind,
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func HaveCurrentDistributionVersion(version string) types.GomegaMatcher {
	return jq.Matchf(`.status.distribution.current.version == "%s"`, version)
}

func HaveTargetDistributionVersion(version string) types.GomegaMatcher {
	return jq.Matchf(`.status.distribution.target.version == "%s"`, version)
}

func HaveTrackedResources() types.GomegaMatcher {
	return gomega.WithTransform(
		jq.Extract(`.status.resources // []`),
		gomega.Not(gomega.BeEmpty()),
	)
}

func HaveNoTrackedResources() types.GomegaMatcher {
	return gomega.WithTransform(
		jq.Extract(`.status.resources // []`),
		gomega.BeEmpty(),
	)
}

func HaveTrackedResource(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return gomega.WithTransform(
		jq.Extract(`.status.resources // []`),
		gomega.ContainElement(gomega.SatisfyAll(
			gomega.HaveKeyWithValue("apiVersion", gvk.GroupVersion().String()),
			gomega.HaveKeyWithValue("kind", gvk.Kind),
		)),
	)
}

func HaveTrackedNamedResource(gvk schema.GroupVersionKind, name string) types.GomegaMatcher {
	return gomega.WithTransform(
		jq.Extract(`.status.resources // []`),
		gomega.ContainElement(gomega.SatisfyAll(
			gomega.HaveKeyWithValue("apiVersion", gvk.GroupVersion().String()),
			gomega.HaveKeyWithValue("kind", gvk.Kind),
			gomega.HaveKeyWithValue("name", name),
		)),
	)
}

func HaveChartInfo(name string, path string) types.GomegaMatcher {
	return gomega.WithTransform(
		jq.Extract(`.status.chart`),
		gomega.SatisfyAll(
			gomega.HaveKeyWithValue("name", name),
			gomega.HaveKeyWithValue("path", path),
		),
	)
}

func HaveRunlevel(runlevel int) types.GomegaMatcher {
	return jq.Matchf(`.status.runlevel == %d`, runlevel)
}
