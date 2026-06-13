//go:build e2e

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

package e2e

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/test/support"
)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	cfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		return 1
	}

	SetDefaultEventuallyTimeout(cfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(cfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(cfg.ConsistentlyPollingInterval)

	return m.Run()
}

func TestOGX(t *testing.T) {
	k8sClient, err := support.NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendatahub-ogx-operator",
			Namespace: support.OperatorNamespace(),
		},
	}
	NewWithT(t).Eventually(t.Context(), k8sm.Get(k8sClient, operatorDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	t.Run("foundation", (&foundationTests{Client: k8sClient}).Execute)
}
