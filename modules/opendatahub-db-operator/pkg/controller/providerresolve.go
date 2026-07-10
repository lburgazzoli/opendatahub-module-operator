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

// Package providerresolve contains the shared DatabaseProvider selection
// helper used by SchemaClaim and DatabaseClaim reconcilers (docs/plan.md §6).
package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

const (
	// AnnotationSelectionPriority is the annotation key on a DatabaseProvider
	// that breaks ties when multiple providers match a selector. Higher integer
	// wins; missing or invalid values are treated as 0.
	AnnotationSelectionPriority = "db.infrastructure.opendatahub.io/selection-priority"
)

// ErrNotFound is returned when no matching DatabaseProvider exists.
type ErrNotFound struct {
	Message string
}

func (e ErrNotFound) Error() string { return e.Message }

// Resolve picks the single best DatabaseProvider for ref. It returns an
// ErrNotFound when no valid provider exists (caller turns this into a Pending
// condition).
//
// Resolution order (docs/plan.md §6):
//  1. ref.Name set → exact Get
//  2. ref.Selector set → List + priority/name sort → winner
func Resolve(
	ctx context.Context,
	cli client.Client,
	ref infraApi.ProviderRef,
) (*infraApi.DatabaseProvider, error) {
	return ResolveForCurrent(ctx, cli, ref, "")
}

// ResolveForCurrent resolves a provider like Resolve, but for selector-based
// references it keeps the currently selected provider when it still matches.
func ResolveForCurrent(
	ctx context.Context,
	cli client.Client,
	ref infraApi.ProviderRef,
	currentProvider string,
) (*infraApi.DatabaseProvider, error) {
	switch {
	case ref.Name != "":
		return resolveByName(ctx, cli, ref.Name)
	case ref.Selector != nil:
		return resolveBySelector(ctx, cli, ref.Selector, currentProvider)
	default:
		return nil, fmt.Errorf("provider reference must set either name or selector")
	}
}

func resolveByName(ctx context.Context, cli client.Client, name string) (*infraApi.DatabaseProvider, error) {
	provider := &infraApi.DatabaseProvider{}
	err := cli.Get(ctx, client.ObjectKey{Name: name}, provider)

	switch {
	case err == nil:
		return provider, nil
	case apierrors.IsNotFound(err):
		return nil, ErrNotFound{Message: fmt.Sprintf("DatabaseProvider %q not found", name)}
	default:
		return nil, fmt.Errorf("getting DatabaseProvider %q: %w", name, err)
	}
}

func resolveBySelector(
	ctx context.Context,
	cli client.Client,
	selector *metav1.LabelSelector,
	currentProvider string,
) (*infraApi.DatabaseProvider, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("building label selector: %w", err)
	}

	if currentProvider != "" {
		current, err := resolveByName(ctx, cli, currentProvider)
		switch {
		case err == nil && labelSelector.Matches(labels.Set(current.Labels)):
			return current, nil
		case err == nil:
			// Current provider still exists but no longer matches; fall through.
		case isNotFound(err):
			// Current provider disappeared; fall through.
		default:
			return nil, err
		}
	}

	list := &infraApi.DatabaseProviderList{}
	if err := cli.List(ctx, list, client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return nil, fmt.Errorf("listing DatabaseProviders by selector: %w", err)
	}

	if len(list.Items) == 0 {
		return nil, ErrNotFound{Message: fmt.Sprintf("no DatabaseProvider matches selector %v", selector)}
	}
	return pickBest(list.Items), nil
}

// pickBest returns the provider with the highest selection-priority annotation
// value, breaking ties alphabetically by name for full determinism.
func pickBest(providers []infraApi.DatabaseProvider) *infraApi.DatabaseProvider {
	sort.Slice(providers, func(i, j int) bool {
		pi := priority(providers[i])
		pj := priority(providers[j])
		if pi != pj {
			return pi > pj
		}
		return providers[i].Name < providers[j].Name
	})

	return providers[0].DeepCopy()
}

func priority(p infraApi.DatabaseProvider) int {
	v, err := strconv.Atoi(p.Annotations[AnnotationSelectionPriority])
	if err != nil {
		return 0
	}
	return v
}

func isNotFound(err error) bool {
	var notFound ErrNotFound
	return errors.As(err, &notFound)
}
