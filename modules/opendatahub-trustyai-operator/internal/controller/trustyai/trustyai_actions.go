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

package trustyai

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	modulegvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/resources/gvk"
	fwcluster "github.com/opendatahub-io/odh-platform-utilities/framework/cluster"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	pkgresources "github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

// checkPreConditions verifies that:
//  1. Kserve module CRD (kserves.components.platform.opendatahub.io) exists — KServe module is installed
//  2. At least one Kserve CR exists — KServe module is enabled
//  3. InferenceServices CRD (inferenceservices.serving.kserve.io) exists — KServe is installed
func (m *Module) checkPreConditions(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	// Check Kserve module CRD — signals the KServe module operator is installed.
	kserveModuleCRD, err := fwcluster.HasCRD(ctx, rr.Client, modulegvk.Kserve)
	switch {
	case err != nil:
		return odherrors.NewStopError("failed to check Kserve module CRD: %w", err)
	case !kserveModuleCRD:
		return odherrors.NewStopError("Kserve module CRD (%s) not found: the KServe module operator must be installed before enabling TrustyAI", modulegvk.Kserve.GroupKind())
	}

	// Check that the Kserve singleton CR exists — signals KServe module is enabled.
	if err := odhcluster.GetSingleton(ctx, rr.Client, pkgresources.GvkToUnstructured(modulegvk.Kserve)); err != nil {
		if k8serr.IsNotFound(err) {
			return odherrors.NewStopError("Kserve CR not found: the KServe module must be enabled before enabling TrustyAI")
		}
		return odherrors.NewStopError("failed to get Kserve CR: %w", err)
	}

	// Check InferenceServices CRD (same check as monolith).
	isvc, err := fwcluster.HasCRD(ctx, rr.Client, modulegvk.InferenceServices)
	switch {
	case err != nil:
		return odherrors.NewStopError("failed to check %s CRD: %w", modulegvk.InferenceServices, err)
	case !isvc:
		return odherrors.NewStopError("InferenceServices CRD (%s) not found: KServe must be installed before enabling TrustyAI", modulegvk.InferenceServices.GroupKind())
	}

	return nil
}

// createConfigMap creates the trustyai-dsc-config ConfigMap with eval permission settings.
// This mirrors the monolith's createConfigMap action (trustyai_controller_actions.go:64).
func (m *Module) createConfigMap(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	tai, ok := rr.Instance.(*componentApi.TrustyAI)
	if !ok {
		return fmt.Errorf("instance is not a TrustyAI")
	}

	permitCodeExecution := tai.Spec.Eval.LMEval.PermitCodeExecution == componentApi.EvalPermissionAllow
	permitOnline := tai.Spec.Eval.LMEval.PermitOnline == componentApi.EvalPermissionAllow

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trustyai-dsc-config",
			Namespace: m.cfg.ApplicationsNamespace,
		},
		Data: map[string]string{
			"eval.lmeval.permitCodeExecution": strconv.FormatBool(permitCodeExecution),
			"eval.lmeval.permitOnline":        strconv.FormatBool(permitOnline),
		},
	}

	return rr.AddResources(configMap)
}
