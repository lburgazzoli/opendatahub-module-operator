//go:build integration

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

package integration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
)

const (
	timeout  = 2 * time.Minute
	interval = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"
)

var _ = Describe("MyModule", Ordered, func() {
	module := &componentsv1alpha1.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.MyModuleInstanceName,
		},
	}

	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mymodule-workload",
			Namespace: testNamespace,
		},
	}

	workloadService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mymodule-workload",
			Namespace: testNamespace,
		},
	}

	AfterAll(func() {
		By("deleting the MyModule CR")
		Eventually(k.Delete(module)).WithContext(ctx).Should(Succeed())
	})

	It("should reconcile the MyModule CR and deploy the workload", func() {
		By("ensuring the MyModule CR does not already exist")
		_ = k8sClient.Delete(ctx, module)

		By("creating the MyModule CR")
		module.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, module)).To(Succeed())

		By("waiting for MyModule to become Ready")
		Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
			jq.Match(`.status.phase == "Ready"`),
			jq.Match(`.status.conditions[] | select(.type == "Ready") | .status == "True"`),
			jq.Match(`.status.conditions[] | select(.type == "ProvisioningSucceeded") | .status == "True"`),
		))

		By("verifying the workload Deployment is available")
		Eventually(k.Get(workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
			jq.Match(`.status.readyReplicas >= 1`),
		)
	})

	It("should expose config values matching the operator ConfigMap", func() {
		By("verifying configValues in status match the source ConfigMap data")
		Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
			jq.Match(`.status.configValues."%s" == "%s"`,
				moduleconfig.KeyPlatformType,
				operatorConfigData[moduleconfig.KeyPlatformType]),
			jq.Match(`.status.configValues."%s" == "%s"`,
				moduleconfig.KeyPlatformVersion,
				operatorConfigData[moduleconfig.KeyPlatformVersion]),
		))
	})

	It("should report module version and platform in status", func() {
		Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
			jq.Match(`.status.module.version != ""`),
			jq.Match(`.status.module.platform.name == "%s"`,
				operatorConfigData[moduleconfig.KeyPlatformType]),
			jq.Match(`.status.module.sources | length > 0`),
			jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
		))
	})

	It("should set platform labels and annotations on workload resources", func() {
		By("checking the Deployment has the part-of label and platform annotations")
		Eventually(k.Get(workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
			jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
			jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceName),
			jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceUID),
			jq.Match(`.metadata.annotations."%s" == "%s"`,
				annotationType,
				operatorConfigData[moduleconfig.KeyPlatformType]),
			jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
		))

		By("checking the Service has the part-of label and platform annotations")
		Eventually(k.Get(workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
			jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
			jq.Match(`.metadata.annotations."%s" == "%s"`,
				annotationType,
				operatorConfigData[moduleconfig.KeyPlatformType]),
			jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
		))
	})

	It("should set owner references on workload resources", func() {
		By("checking the Deployment has an owner reference to the MyModule CR")
		Eventually(k.Get(workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
			jq.Match(`.metadata.ownerReferences[] | select(.kind == "MyModule") | .name == "%s"`,
				componentsv1alpha1.MyModuleInstanceName),
		)

		By("checking the Service has an owner reference to the MyModule CR")
		Eventually(k.Get(workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
			jq.Match(`.metadata.ownerReferences[] | select(.kind == "MyModule") | .name == "%s"`,
				componentsv1alpha1.MyModuleInstanceName),
		)
	})
})
