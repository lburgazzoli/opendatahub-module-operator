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
	"context"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster"
)

const (
	operatorDeploymentName  = "odh-db-operator-operator"
	operatorConfigMapName   = "odh-db-operator-config"
	defaultE2ETestNamespace = "odh-db-operator-e2e"
)

func TestDatabaseOperatorE2E(t *testing.T) {
	cfg := loadE2EConfig(t)
	suite := newE2ESuite(t, cfg)

	t.Run("foundation", suite.runFoundation)
	t.Run("internal", suite.runInternal)
	t.Run("external", suite.runExternal)
}

func newE2ESuite(
	t *testing.T,
	cfg *support.Config,
) *e2eSuite {
	g := NewWithT(t)

	if cfg == nil {
		t.Fatal("failed to load e2e config")
	}

	testCluster, err := cluster.New(t.Context(), cfg, cluster.WithLogFn(t.Logf))
	if err != nil {
		t.Fatalf("failed to create e2e cluster: %v", err)
	}
	if err := testCluster.Setup(t.Context()); err != nil {
		t.Fatalf("failed to setup e2e cluster: %v", err)
	}
	t.Cleanup(func() {
		_ = testCluster.Stop(context.Background())
	})

	cli := testCluster.Client()
	if cli == nil {
		t.Fatal("failed to create client")
	}

	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorDeploymentName,
			Namespace: cfg.Operator.Namespace,
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(cli, operatorDeploy)).Should(Succeed())
	g.Eventually(t.Context(), k8sm.Get(cli, operatorDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	return &e2eSuite{
		Client:            cli,
		operatorNamespace: cfg.Operator.Namespace,
		workloadNamespace: e2eTestNamespace(),
	}
}

func loadE2EConfig(t *testing.T) *support.Config {
	t.Helper()

	cfg, err := support.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load e2e config: %v", err)
	}

	SetDefaultEventuallyTimeout(cfg.Gomega.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(cfg.Gomega.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(cfg.Gomega.ConsistentlyPollingInterval)

	return cfg
}

type e2eSuite struct {
	Client            client.Client
	operatorNamespace string
	workloadNamespace string
}
