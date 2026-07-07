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

// Package integration holds this module's integration test suite, run against
// the connected cluster (docs/plan.md §11 "Definition of done").
package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

func TestMain(m *testing.M) {
	gomegaCfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		os.Exit(1)
	}

	SetDefaultEventuallyTimeout(gomegaCfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(gomegaCfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(gomegaCfg.ConsistentlyPollingInterval)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	os.Exit(m.Run())
}

// TestManagerStartup is task-01's integration smoke test (docs/task-01.md step 7):
// the scaffold must actually run against the connected cluster, not just build.
// No CRDs exist yet at this stage (task-02 adds them) -- this only proves the
// manager reaches a healthy state.
func TestManagerStartup(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	restCfg, err := config.GetConfig()
	g.Expect(err).NotTo(HaveOccurred(), "connected cluster's kubeconfig must be reachable")

	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())

	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "127.0.0.1:18081"
	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Pprof.BindAddress = "0"

	mgr, err := modulemanager.New(ctx, restCfg, moduleCfg)
	g.Expect(err).NotTo(HaveOccurred())

	mgrDone := make(chan error, 1)
	go func() {
		mgrDone <- mgr.Start(ctx)
	}()

	g.Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue(), "manager cache must sync against the connected cluster")

	// https://onsi.github.io/gomega/#codeghttpcode-testing-http-clients --
	// Eventually polls the HTTP call directly and HaveHTTPStatus asserts on
	// the response, instead of a manual status-code Expect inside the poll.
	g.Eventually(func() (*http.Response, error) {
		return http.Get("http://127.0.0.1:18081/healthz")
	}).Should(HaveHTTPStatus(http.StatusOK), "/healthz must respond once the manager is up")

	g.Eventually(func() (*http.Response, error) {
		return http.Get("http://127.0.0.1:18081/readyz")
	}).Should(HaveHTTPStatus(http.StatusOK), "/readyz must respond once the manager is up")

	cancel()
	g.Eventually(mgrDone).Should(Receive())
}
