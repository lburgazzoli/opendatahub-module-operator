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

package databaseprovider

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbclaimcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	schemaclaimcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	// instanceHashAnnotation holds an opaque random token on the admin Secret.
	// It is generated once on Secret creation, preserved on subsequent reconciles,
	// and changes only when the Secret is deleted and recreated. The token is
	// written into the StatefulSet pod-template annotation so that any Secret
	// recreation triggers a rolling restart — without exposing the password.
	instanceHashAnnotation = dbcontroller.EmbeddedInstanceHashAnnotation

	// extKeyInstanceHash is the rr.Extensions key that carries the credential ID
	// between reconcileEmbeddedAction and embeddedTemplateData.
	extKeyInstanceHash = "embedded/instanceHash"

	reasonAdminSecretUnavailable = "AdminSecretUnavailable"
	reasonInstanceRunning        = "InstanceRunning"
	reasonProvisioning           = "Provisioning"
	reasonIdle                   = "Idle"
	defaultEmbeddedAdminUser     = "postgres"
	defaultEmbeddedAdminDatabase = "postgres"
)

var (
	stockExtensions = map[string]struct{}{
		"pg_trgm":   {},
		"uuid_ossp": {},
		"pgcrypto":  {},
	}
)

func embeddedTemplateData(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
	cfg *moduleconfig.Config,
) (map[string]any, error) {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok || obj.Spec.Type != infraApi.ProviderTypeEmbedded {
		return nil, nil
	}

	namespaces, err := referencedClaimNamespaces(ctx, rr.Client, obj)
	if err != nil {
		return nil, fmt.Errorf("listing referenced claim namespaces: %w", err)
	}

	resolvedImage, err := resolveEmbeddedImage(obj, cfg)
	if err != nil {
		return nil, err
	}

	credHash, _ := rr.Extensions[extKeyInstanceHash].(string)

	return map[string]any{
		"ResolvedImage":       resolvedImage,
		"AdminSecretName":     dbcontroller.EmbeddedAdminSecretName(obj.Name),
		"ServiceName":         dbcontroller.EmbeddedServiceName(obj.Name),
		"PVCName":             dbcontroller.EmbeddedPVCName(obj.Name),
		"InitDBConfigMapName": dbcontroller.EmbeddedInitDBConfigMapName(obj.Name),
		"AllowedNamespaces":   namespaces,
		"InstanceHash":        credHash,
	}, nil
}

func resolveEmbeddedImage(obj *infraApi.DatabaseProvider, cfg *moduleconfig.Config) (string, error) {
	if obj.Spec.Embedded == nil {
		return "", fmt.Errorf("spec.embedded is required for Embedded providers")
	}

	hasVector := false
	for _, extension := range obj.Spec.Embedded.Extensions {
		if extension == "vector" {
			hasVector = true
			continue
		}
		if _, ok := stockExtensions[extension]; !ok {
			return "", fmt.Errorf(
				"extension %q does not map to a supported embedded image; use an External provider",
				extension,
			)
		}
	}

	switch {
	case hasVector:
		return cfg.Embedded.PgvectorImage, nil
	default:
		return cfg.Embedded.PostgresImage, nil
	}
}

// computeEmbeddedAdminSecret returns the desired admin Secret for the embedded
// provider. If the Secret already exists it is returned as-is. If it is absent
// fresh credentials are generated. The caller adds it to rr.Resources so the
// deploy action creates or updates it via SSA.
func computeEmbeddedAdminSecret(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (*corev1.Secret, error) {

	existing := &corev1.Secret{}
	existing.Name = dbcontroller.EmbeddedAdminSecretName(provider.Name)
	existing.Namespace = dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace)

	err := cli.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	switch {
	case err == nil:
		// Secret found — project a clean desired state with only the fields we own.
		// Returning existing as-is would carry over server-side metadata that SSA
		// should not manage (ResourceVersion, owner references from other actors, etc.).
		res := &corev1.Secret{}
		res.Name = existing.Name
		res.Namespace = existing.Namespace
		res.Annotations = map[string]string{
			instanceHashAnnotation: existing.Annotations[instanceHashAnnotation],
		}
		res.Data = maps.Clone(existing.Data)

		return res, nil
	case apierrors.IsNotFound(err):
		password, err := postgres.GeneratePassword(24)
		if err != nil {
			return nil, fmt.Errorf("generating admin password: %w", err)
		}

		existing.Annotations = map[string]string{
			instanceHashAnnotation: xid.New().String(),
		}

		existing.Data = map[string][]byte{
			postgres.SecretKeyHost:     []byte(dbcontroller.EmbeddedServiceHost(provider, cfg.OperatorNamespace)),
			postgres.SecretKeyPort:     fmt.Appendf(nil, "%d", postgres.DefaultPort),
			postgres.SecretKeyUser:     []byte(defaultEmbeddedAdminUser),
			postgres.SecretKeyPassword: []byte(password),
			postgres.SecretKeyDatabase: []byte(defaultEmbeddedAdminDatabase),
		}

		return existing, nil
	default:
		return nil, err
	}
}

func referencedClaimNamespaces(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
) ([]string, error) {
	namespaces := map[string]struct{}{}

	schemaClaims := &infraApi.SchemaClaimList{}
	if err := cli.List(ctx, schemaClaims); err != nil {
		return nil, err
	}
	for i := range schemaClaims.Items {
		claim := &schemaClaims.Items[i]
		if schemaClaimEffectiveProvider(claim) != provider.Name {
			continue
		}
		if isConditionTrue(claim.Status.Conditions, schemaclaimcontroller.ConditionProvisioned) {
			namespaces[claim.Namespace] = struct{}{}
		}
	}

	databaseClaims := &infraApi.DatabaseClaimList{}
	if err := cli.List(ctx, databaseClaims); err != nil {
		return nil, err
	}
	for i := range databaseClaims.Items {
		claim := &databaseClaims.Items[i]
		if databaseClaimEffectiveProvider(claim) != provider.Name {
			continue
		}
		if isConditionTrue(claim.Status.Conditions, dbclaimcontroller.ConditionProvisioned) {
			namespaces[claim.Namespace] = struct{}{}
		}
	}

	result := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		result = append(result, namespace)
	}
	sort.Strings(result)
	return result, nil
}

func schemaClaimEffectiveProvider(claim *infraApi.SchemaClaim) string {
	if claim.Spec.Provider.Name != "" {
		return claim.Spec.Provider.Name
	}
	return claim.Status.Provider
}

func databaseClaimEffectiveProvider(claim *infraApi.DatabaseClaim) string {
	if claim.Spec.Provider.Name != "" {
		return claim.Spec.Provider.Name
	}
	return claim.Status.Provider
}

func embeddedStatefulSetKey(provider *infraApi.DatabaseProvider, cfg *moduleconfig.Config) types.NamespacedName {
	operatorNamespace := cfg.OperatorNamespace
	return types.NamespacedName{
		Namespace: dbcontroller.EmbeddedNamespace(provider, operatorNamespace),
		Name:      dbcontroller.EmbeddedServiceName(provider.Name),
	}
}

func embeddedChildResources(provider *infraApi.DatabaseProvider, cfg *moduleconfig.Config) []client.Object {
	namespace := dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace)
	return []client.Object{
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: dbcontroller.EmbeddedServiceName(provider.Name)}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: dbcontroller.EmbeddedPVCName(provider.Name)}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: dbcontroller.EmbeddedServiceName(provider.Name)}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: dbcontroller.EmbeddedInitDBConfigMapName(provider.Name)}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: dbcontroller.EmbeddedServiceName(provider.Name)}},
	}
}

func deleteEmbeddedChildResources(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) error {
	for _, obj := range embeddedChildResources(provider, cfg) {
		if err := cli.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func shouldTearDownIdleInstance(provider *infraApi.DatabaseProvider, gracePeriod time.Duration) bool {
	condition := findCondition(provider.Status.Conditions, ConditionReachable)
	if condition == nil || condition.Reason != reasonIdle {
		return false
	}
	return time.Since(condition.LastTransitionTime.Time) >= gracePeriod
}

func wantsIdleDeletion(provider *infraApi.DatabaseProvider) bool {
	if provider.Spec.Embedded == nil {
		return false
	}
	if provider.Spec.Embedded.DeletionPolicy == "" {
		return false
	}
	return provider.Spec.Embedded.DeletionPolicy == infraApi.DeletionPolicyDelete
}

func isConditionTrue(conditions []common.Condition, conditionType string) bool {
	condition := findCondition(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func findCondition(conditions []common.Condition, conditionType string) *common.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
