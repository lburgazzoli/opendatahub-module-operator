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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	ConditionReachable = "Reachable"

	ConditionTLSConfiguration = dbcontroller.ConditionTLSConfiguration
)

type ErrConnectionSecretUnavailable struct {
	Ref   corev1.SecretReference
	Cause error
}

func (e ErrConnectionSecretUnavailable) Error() string {
	return fmt.Sprintf("reading connection Secret %s/%s: %v", e.Ref.Namespace, e.Ref.Name, e.Cause)
}

func (e ErrConnectionSecretUnavailable) Unwrap() error {
	return e.Cause
}

type ErrConnectionSecretInvalid struct {
	Name  string
	Cause error
}

func (e ErrConnectionSecretInvalid) Error() string {
	return fmt.Sprintf("invalid connection Secret %q: %v", e.Name, e.Cause)
}

func (e ErrConnectionSecretInvalid) Unwrap() error {
	return e.Cause
}

func loadExternalConfig(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
) (postgres.Config, error) {
	if provider.Spec.External == nil {
		return postgres.Config{}, ErrConnectionSecretInvalid{
			Name:  provider.Name,
			Cause: fmt.Errorf("spec.external is required for External providers"),
		}
	}

	ref := provider.Spec.External.ConnectionSecretRef
	secret := &corev1.Secret{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, secret); err != nil {
		return postgres.Config{}, ErrConnectionSecretUnavailable{
			Ref:   ref,
			Cause: err,
		}
	}

	cfg, err := postgres.ParseSecret(secret.Data)
	if err != nil {
		return postgres.Config{}, ErrConnectionSecretInvalid{
			Name:  fmt.Sprintf("%s/%s", secret.Namespace, secret.Name),
			Cause: err,
		}
	}

	return cfg, nil
}

func externalFailureReason(err error) string {
	if _, ok := errors.AsType[ErrConnectionSecretUnavailable](err); ok {
		return "ConnectionSecretUnavailable"
	}
	if _, ok := errors.AsType[ErrConnectionSecretInvalid](err); ok {
		return "ConnectionSecretInvalid"
	}
	return "ConnectionCheckFailed"
}

func providerConnectionStatus(cfg postgres.Config) infraApi.ProviderConnectionStatus {
	return infraApi.ProviderConnectionStatus{
		Host:     cfg.Host,
		Port:     int32(cfg.Port),
		Database: cfg.DBName,
	}
}
