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
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	dbmaps "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/utils/maps"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	// extKeyAdminSecretKey is the rr.Extensions key that carries the embedded
	// admin Secret rollout token between reconcileEmbeddedAction and
	// embeddedTemplateData.
	extKeyAdminSecretKey = "embedded/adminSecretKey"
	extKeyTLSSecretHash  = "embedded/tlsSecretHash"

	reasonAdminSecretUnavailable = "AdminSecretUnavailable"
	reasonInstanceRunning        = "InstanceRunning"
	reasonProvisioning           = "Provisioning"
	reasonTLSNotEnabled          = "TLSNotEnabled"
	reasonTLSProvisioning        = "TLSProvisioning"
	reasonTLSConfigured          = "TLSConfigured"
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

type TLSState struct {
	Enabled       bool
	Ready         bool
	TLSSecret     *corev1.Secret
	TLSSecretHash string
	CAData        []byte
	AdminSecret   *corev1.Secret
}

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

	adminSecretKey, _ := rr.Extensions[extKeyAdminSecretKey].(string)
	tlsHash, _ := rr.Extensions[extKeyTLSSecretHash].(string)

	return map[string]any{
		"ResolvedImage":        resolvedImage,
		"AdminSecretName":      dbcontroller.EmbeddedAdminSecretName(obj.Name),
		"ServiceName":          dbcontroller.EmbeddedServiceName(obj.Name),
		"PVCName":              dbcontroller.EmbeddedPVCName(obj.Name),
		"InitDBConfigMapName":  dbcontroller.EmbeddedInitDBConfigMapName(obj.Name),
		"TLSIssuerName":        dbcontroller.EmbeddedTLSIssuerName(obj.Name),
		"TLSCertificateName":   embeddedTLSCertificateName(obj),
		"TLSIssuerRef":         embeddedTLSIssuerRef(obj),
		"AllowedNamespaces":    namespaces,
		"InstanceHash":         adminSecretKey,
		"TLSEnabled":           embeddedTLSEnabled(obj),
		"TLSUsesManagedIssuer": embeddedTLSUsesManagedIssuer(obj),
		"TLSSecretName":        embeddedTLSSecretName(obj),
		"TLSSecretHash":        tlsHash,
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

func embeddedTLSEnabled(provider *infraApi.DatabaseProvider) bool {
	return provider != nil && provider.Spec.Embedded != nil && provider.Spec.Embedded.TLS != nil
}

func embeddedTLSUsesManagedIssuer(provider *infraApi.DatabaseProvider) bool {
	if !embeddedTLSEnabled(provider) {
		return false
	}

	ref := provider.Spec.Embedded.TLS.IssuerRef
	return ref == nil || ref.Name == ""
}

func embeddedTLSSecretName(provider *infraApi.DatabaseProvider) string {
	if !embeddedTLSEnabled(provider) {
		return ""
	}

	if name := provider.Spec.Embedded.TLS.Certificate.SecretName; name != "" {
		return name
	}

	return dbcontroller.EmbeddedTLSSecretName(provider.Name)
}

func embeddedTLSCertificateName(provider *infraApi.DatabaseProvider) string {
	if provider == nil {
		return ""
	}

	return dbcontroller.EmbeddedTLSCertificateName(provider.Name)
}

func embeddedTLSIssuerRef(provider *infraApi.DatabaseProvider) *infraApi.CertManagerIssuerRef {
	if !embeddedTLSEnabled(provider) {
		return nil
	}

	if embeddedTLSUsesManagedIssuer(provider) {
		return &infraApi.CertManagerIssuerRef{
			Name:  dbcontroller.EmbeddedTLSIssuerName(provider.Name),
			Kind:  gvk.CertManagerIssuer.Kind,
			Group: gvk.CertManagerIssuer.Group,
		}
	}

	ref := *provider.Spec.Embedded.TLS.IssuerRef
	if ref.Kind == "" {
		ref.Kind = gvk.CertManagerIssuer.Kind
	}
	if ref.Group == "" {
		ref.Group = gvk.CertManagerIssuer.Group
	}

	return &ref
}

func embeddedTLSStatus(
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
	ready bool,
) *infraApi.ProviderTLSStatus {
	if provider == nil {
		return nil
	}

	return &infraApi.ProviderTLSStatus{
		Enabled:         embeddedTLSEnabled(provider),
		Ready:           ready,
		Namespace:       dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace),
		IssuerRef:       embeddedTLSIssuerRef(provider),
		CertificateName: embeddedTLSCertificateName(provider),
		SecretName:      embeddedTLSSecretName(provider),
	}
}

func embeddedTLSSecret(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (*corev1.Secret, error) {
	if !embeddedTLSEnabled(provider) {
		return nil, nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      embeddedTLSSecretName(provider),
			Namespace: dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace),
		},
	}
	if err := cli.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("reading embedded TLS Secret: %w", err)
	}

	return secret, nil
}

func embeddedTLSCAData(secret *corev1.Secret) []byte {
	if secret == nil {
		return nil
	}

	if ca := secret.Data[postgres.SecretKeyCA]; len(ca) != 0 {
		return ca
	}

	return secret.Data["tls.crt"]
}

func embeddedTLSSecretHash(secret *corev1.Secret) string {
	if secret == nil {
		return ""
	}

	h := sha256.New()
	keys := []string{"ca.crt", "tls.crt", "tls.key"}
	for _, key := range keys {
		if value := secret.Data[key]; len(value) != 0 {
			_, _ = h.Write([]byte(key))
			_, _ = h.Write(value)
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

func resolveEmbeddedTLSState(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (TLSState, error) {
	state := TLSState{
		Enabled: embeddedTLSEnabled(provider),
	}
	if !state.Enabled {
		return state, nil
	}

	secret, err := embeddedTLSSecret(ctx, cli, provider, cfg)
	if err != nil {
		return state, err
	}
	if secret == nil {
		return state, nil
	}

	state.TLSSecret = secret
	state.CAData = append([]byte(nil), embeddedTLSCAData(secret)...)
	state.Ready = len(state.CAData) != 0
	state.TLSSecretHash = embeddedTLSSecretHash(secret)

	return state, nil
}

// computeEmbeddedAdminSecret returns the resolved TLS state for the embedded
// provider, including the desired admin Secret projection. If the admin Secret
// already exists its password is preserved; otherwise fresh credentials are
// generated. The caller adds TLSState.AdminSecret to rr.Resources so the deploy
// action creates or updates it via SSA.
func computeEmbeddedAdminSecret(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (TLSState, error) {
	res, err := resolveEmbeddedTLSState(ctx, cli, provider, cfg)
	if err != nil {
		return res, err
	}

	existing := &corev1.Secret{}
	existing.Name = dbcontroller.EmbeddedAdminSecretName(provider.Name)
	existing.Namespace = dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace)

	err = cli.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return res, err
	}

	password := existing.Data[postgres.SecretKeyPassword]
	if len(password) == 0 {
		pwd, err := postgres.GeneratePassword(24)
		if err != nil {
			return res, fmt.Errorf("generating admin password: %w", err)
		}

		password = []byte(pwd)
	}

	instanceHash := existing.Annotations[dbcontroller.EmbeddedAdminSecretKeyAnnotation]
	if len(instanceHash) == 0 {
		instanceHash = xid.New().String()
	}

	res.AdminSecret = desiredEmbeddedAdminSecret(provider, cfg, password, res)
	res.AdminSecret.Annotations = dbmaps.Set(
		res.AdminSecret.Annotations,
		dbcontroller.EmbeddedAdminSecretKeyAnnotation,
		instanceHash,
	)

	return res, nil
}

func desiredEmbeddedAdminSecret(
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
	password []byte,
	tlsState TLSState,
) *corev1.Secret {
	res := &corev1.Secret{}
	res.Name = dbcontroller.EmbeddedAdminSecretName(provider.Name)
	res.Namespace = dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace)
	res.Data = map[string][]byte{
		postgres.SecretKeyHost:     []byte(dbcontroller.EmbeddedServiceHost(provider, cfg.OperatorNamespace)),
		postgres.SecretKeyPort:     fmt.Appendf(nil, "%d", postgres.DefaultPort),
		postgres.SecretKeyUser:     []byte(defaultEmbeddedAdminUser),
		postgres.SecretKeyPassword: password,
		postgres.SecretKeyDatabase: []byte(defaultEmbeddedAdminDatabase),
	}

	if !embeddedTLSEnabled(provider) {
		return res
	}

	res.Data[postgres.SecretKeySSLMode] = []byte(postgres.SSLModeVerifyFull)
	if len(tlsState.CAData) != 0 {
		res.Data[postgres.SecretKeyCA] = append([]byte(nil), tlsState.CAData...)
	}

	return res
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
		if conditions.IsStatusConditionTrue(claim, dbcontroller.ConditionProvisioned) {
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
		if conditions.IsStatusConditionTrue(claim, dbcontroller.ConditionProvisioned) {
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
