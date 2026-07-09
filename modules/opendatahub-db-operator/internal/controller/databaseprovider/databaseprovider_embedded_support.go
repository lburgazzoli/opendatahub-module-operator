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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbclaimcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	schemaclaimcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	reasonConnectionVerified              = "ConnectionVerified"
	reasonConnectionCheckFailed           = "ConnectionCheckFailed"
	reasonImageUnmapped                   = "ImageUnmapped"
	reasonExtensionChangeRequiresRecreate = "ExtensionChangeRequiresRecreate"
	reasonAdminSecretUnavailable          = "AdminSecretUnavailable"
	reasonInstanceRunning                 = "InstanceRunning"
	reasonProvisioning                    = "Provisioning"
	reasonIdle                            = "Idle"
	capabilityLabelPrefix                 = "db.infrastructure.opendatahub.io/capability-"
	defaultEmbeddedAdminUser              = "postgres"
	defaultEmbeddedAdminDatabase          = "postgres"
)

var (
	stockExtensions = map[string]struct{}{
		"pg_trgm":   {},
		"uuid_ossp": {},
		"pgcrypto":  {},
	}
	capabilityLabelByExtension = map[string]string{
		"vector":    capabilityLabelPrefix + "pgvector",
		"pg_trgm":   capabilityLabelPrefix + "pg_trgm",
		"uuid_ossp": capabilityLabelPrefix + "uuid_ossp",
		"pgcrypto":  capabilityLabelPrefix + "pgcrypto",
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

	return map[string]any{
		"ResolvedImage":       resolvedImage,
		"AdminSecretName":     dbcontroller.EmbeddedAdminSecretName(obj.Name),
		"ServiceName":         dbcontroller.EmbeddedServiceName(obj.Name),
		"PVCName":             dbcontroller.EmbeddedPVCName(obj.Name),
		"InitDBConfigMapName": dbcontroller.EmbeddedInitDBConfigMapName(obj.Name),
		"AllowedNamespaces":   namespaces,
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

func ensureEmbeddedAdminSecret(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (*corev1.Secret, error) {
	operatorNamespace := cfg.OperatorNamespace
	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Namespace: dbcontroller.EmbeddedNamespace(provider, operatorNamespace),
		Name:      dbcontroller.EmbeddedAdminSecretName(provider.Name),
	}
	if err := cli.Get(ctx, key, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		exists, existsErr := embeddedInstanceExists(ctx, cli, provider, cfg)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, fmt.Errorf("embedded admin Secret %s/%s not found for an existing instance", key.Namespace, key.Name)
		}

		password, pwErr := postgres.GeneratePassword(24)
		if pwErr != nil {
			return nil, fmt.Errorf("generating admin password: %w", pwErr)
		}

		secret = buildEmbeddedAdminSecret(key, password)
		if err := ctrl.SetControllerReference(provider, secret, cli.Scheme()); err != nil {
			return nil, fmt.Errorf("setting admin Secret owner reference: %w", err)
		}
		if err := cli.Create(ctx, secret); err != nil {
			return nil, err
		}
		return secret, nil
	}

	if embeddedAdminSecretComplete(secret) {
		return secret, nil
	}

	exists, err := embeddedInstanceExists(ctx, cli, provider, cfg)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("embedded admin Secret %s/%s is incomplete for an existing instance", key.Namespace, key.Name)
	}

	password, err := postgres.GeneratePassword(24)
	if err != nil {
		return nil, fmt.Errorf("generating admin password: %w", err)
	}

	base := secret.DeepCopy()
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		dbcontroller.EmbeddedAdminSecretUserKey:     []byte(defaultEmbeddedAdminUser),
		dbcontroller.EmbeddedAdminSecretPasswordKey: []byte(password),
		dbcontroller.EmbeddedAdminSecretDBKey:       []byte(defaultEmbeddedAdminDatabase),
	}
	if err := ctrl.SetControllerReference(provider, secret, cli.Scheme()); err != nil {
		return nil, fmt.Errorf("setting admin Secret owner reference: %w", err)
	}
	if err := cli.Patch(ctx, secret, client.MergeFrom(base)); err != nil {
		return nil, err
	}
	return secret, nil
}

func embeddedAdminSecretComplete(secret *corev1.Secret) bool {
	if secret == nil {
		return false
	}

	return len(secret.Data[dbcontroller.EmbeddedAdminSecretUserKey]) != 0 &&
		len(secret.Data[dbcontroller.EmbeddedAdminSecretPasswordKey]) != 0 &&
		len(secret.Data[dbcontroller.EmbeddedAdminSecretDBKey]) != 0
}

func buildEmbeddedAdminSecret(key types.NamespacedName, password string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       gvk.Secret.Kind,
			APIVersion: gvk.Secret.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: key.Namespace,
			Name:      key.Name,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			dbcontroller.EmbeddedAdminSecretUserKey:     []byte(defaultEmbeddedAdminUser),
			dbcontroller.EmbeddedAdminSecretPasswordKey: []byte(password),
			dbcontroller.EmbeddedAdminSecretDBKey:       []byte(defaultEmbeddedAdminDatabase),
		},
	}
}

func desiredCapabilityLabels(provider *infraApi.DatabaseProvider) map[string]string {
	labels := map[string]string{}
	for _, extension := range provider.Spec.Embedded.Extensions {
		label, ok := capabilityLabelByExtension[extension]
		if ok {
			labels[label] = "true"
		}
	}
	return labels
}

func currentManagedCapabilityLabels(provider *infraApi.DatabaseProvider) map[string]string {
	labels := map[string]string{}
	for key, value := range provider.Labels {
		if len(key) >= len(capabilityLabelPrefix) && key[:len(capabilityLabelPrefix)] == capabilityLabelPrefix {
			labels[key] = value
		}
	}
	return labels
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
		if !isConditionTrue(claim.Status.Conditions, schemaclaimcontroller.ConditionProvisioned) {
			continue
		}
		matched, err := claimReferencesProvider(ctx, cli, claim.Spec.Provider, provider.Name)
		if err != nil {
			return nil, err
		}
		if matched {
			namespaces[claim.Namespace] = struct{}{}
		}
	}

	databaseClaims := &infraApi.DatabaseClaimList{}
	if err := cli.List(ctx, databaseClaims); err != nil {
		return nil, err
	}
	for i := range databaseClaims.Items {
		claim := &databaseClaims.Items[i]
		if !isConditionTrue(claim.Status.Conditions, dbclaimcontroller.ConditionProvisioned) {
			continue
		}
		matched, err := claimReferencesProvider(ctx, cli, claim.Spec.Provider, provider.Name)
		if err != nil {
			return nil, err
		}
		if matched {
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

func claimReferencesProvider(
	ctx context.Context,
	cli client.Client,
	ref infraApi.ProviderRef,
	providerName string,
) (bool, error) {
	resolved, err := dbcontroller.Resolve(ctx, cli, ref)
	if err != nil {
		if _, ok := err.(dbcontroller.ErrNotFound); ok {
			return false, nil
		}
		return false, err
	}
	return resolved.Name == providerName, nil
}

func embeddedStatefulSetKey(provider *infraApi.DatabaseProvider, cfg *moduleconfig.Config) types.NamespacedName {
	operatorNamespace := cfg.OperatorNamespace
	return types.NamespacedName{
		Namespace: dbcontroller.EmbeddedNamespace(provider, operatorNamespace),
		Name:      dbcontroller.EmbeddedServiceName(provider.Name),
	}
}

func embeddedInstanceExists(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (bool, error) {
	sts := &appsv1.StatefulSet{}
	if err := cli.Get(ctx, embeddedStatefulSetKey(provider, cfg), sts); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

func hasExtensionChange(provider *infraApi.DatabaseProvider) bool {
	current := currentManagedCapabilityLabels(provider)
	desired := desiredCapabilityLabels(provider)
	return len(current) != 0 && !maps.Equal(current, desired)
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
