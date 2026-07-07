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
	"fmt"
	"sort"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

const (
	// AnnotationSelectionPriority is the annotation key on a DatabaseProvider
	// that breaks ties when multiple providers match a selector. Higher integer
	// wins; missing or invalid values are treated as 0.
	AnnotationSelectionPriority = "db.infrastructure.opendatahub.io/selection-priority"

	// AnnotationIsDefault marks a DatabaseProvider as the fallback when a
	// claim specifies neither spec.provider.name nor spec.provider.selector.
	AnnotationIsDefault = "db.infrastructure.opendatahub.io/is-default-provider"
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
//  3. neither set → List + filter is-default-provider annotation
func Resolve(
	ctx context.Context,
	cli client.Client,
	ref infraApi.ProviderRef,
) (*infraApi.DatabaseProvider, error) {
	switch {
	case ref.Name != "":
		return resolveByName(ctx, cli, ref.Name)
	case ref.Selector != nil:
		return resolveBySelector(ctx, cli, ref.Selector)
	default:
		return resolveDefault(ctx, cli)
	}
}

func resolveByName(ctx context.Context, cli client.Client, name string) (*infraApi.DatabaseProvider, error) {
	p := &infraApi.DatabaseProvider{}
	if err := cli.Get(ctx, client.ObjectKey{Name: name}, p); err != nil {
		return nil, ErrNotFound{Message: fmt.Sprintf("DatabaseProvider %q not found: %v", name, err)}
	}
	return p, nil
}

func resolveBySelector(
	ctx context.Context,
	cli client.Client,
	selector *metav1.LabelSelector,
) (*infraApi.DatabaseProvider, error) {
	list := &infraApi.DatabaseProviderList{}
	if err := cli.List(ctx, list, client.MatchingLabels(selector.MatchLabels)); err != nil {
		return nil, fmt.Errorf("listing DatabaseProviders by selector: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, ErrNotFound{Message: fmt.Sprintf("no DatabaseProvider matches selector %v", selector.MatchLabels)}
	}
	return pickBest(list.Items), nil
}

func resolveDefault(ctx context.Context, cli client.Client) (*infraApi.DatabaseProvider, error) {
	list := &infraApi.DatabaseProviderList{}
	if err := cli.List(ctx, list); err != nil {
		return nil, fmt.Errorf("listing DatabaseProviders: %w", err)
	}

	var defaults []infraApi.DatabaseProvider
	for _, p := range list.Items {
		if p.Annotations[AnnotationIsDefault] == "true" {
			defaults = append(defaults, p)
		}
	}
	if len(defaults) == 0 {
		return nil, ErrNotFound{Message: "no DatabaseProvider is annotated as the default"}
	}
	return pickBest(defaults), nil
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
