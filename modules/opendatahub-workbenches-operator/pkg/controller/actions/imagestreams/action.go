package imagestreams

import (
	"context"
	"fmt"
	"maps"
	"strings"

	imagev1 "github.com/openshift/api/image/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
	platform "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/platform"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

const (
	maxConditionMessageLen = 100
	maxFailedTags          = 10
)

type Action struct {
	labels      map[string]string
	namespaceFn actions.Getter[string]
}

type ActionOpts func(*Action)

func WithSelectorLabel(k string, v string) ActionOpts {
	return func(action *Action) {
		action.labels[k] = v
	}
}

func WithSelectorLabels(values map[string]string) ActionOpts {
	return func(action *Action) {
		maps.Copy(action.labels, values)
	}
}

func InNamespace(ns string) ActionOpts {
	return func(action *Action) {
		action.namespaceFn = func(_ context.Context, _ *types.ReconciliationRequest) (string, error) {
			return ns, nil
		}
	}
}

func (a *Action) run(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(types.ResourceObject)
	if !ok {
		return fmt.Errorf("resource instance %v is not a ResourceObject", rr.Instance)
	}

	l := make(map[string]string, len(a.labels))
	maps.Copy(l, a.labels)

	if l[platform.PartOfLabel] == "" {
		kind, err := resources.KindForObject(rr.Client.Scheme(), rr.Instance)
		if err != nil {
			return err
		}

		l[platform.PartOfLabel] = strings.ToLower(kind)
	}

	imageStreams := &imagev1.ImageStreamList{}

	ns, err := a.namespaceFn(ctx, rr)
	if err != nil {
		return fmt.Errorf("unable to compute namespace: %w", err)
	}

	err = rr.Client.List(
		ctx,
		imageStreams,
		client.InNamespace(ns),
		client.MatchingLabels(l),
	)

	if meta.IsNoMatchError(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("error fetching list of ImageStreams: %w", err)
	}

	s := obj.GetStatus()
	rr.Conditions.MarkTrue(
		module.ConditionImageStreamsAvailable,
		conditions.WithObservedGeneration(s.ObservedGeneration),
	)

	if len(imageStreams.Items) == 0 {
		return nil
	}

	var failedTags []string

	for i := range imageStreams.Items {
		is := &imageStreams.Items[i]
		for _, tagStatus := range is.Status.Tags {
			if len(tagStatus.Items) > 0 {
				continue
			}
			for _, cond := range tagStatus.Conditions {
				if cond.Type == imagev1.ImportSuccess && cond.Status == corev1.ConditionFalse {
					msg := cond.Message
					if len(msg) > maxConditionMessageLen {
						msg = msg[:maxConditionMessageLen] + "..."
					}
					failedTags = append(failedTags, fmt.Sprintf("%s:%s (%s)", is.Name, tagStatus.Tag, msg))
				}
			}
		}
	}

	if len(failedTags) > 0 {
		reported := failedTags
		suffix := ""
		if len(reported) > maxFailedTags {
			suffix = fmt.Sprintf("; ... and %d more", len(reported)-maxFailedTags)
			reported = reported[:maxFailedTags]
		}

		rr.Conditions.MarkFalse(
			module.ConditionImageStreamsAvailable,
			conditions.WithObservedGeneration(s.ObservedGeneration),
			conditions.WithReason(module.ConditionImageStreamsNotAvailableReason),
			conditions.WithMessage(
				"Warning: %d ImageStream tag(s) failed to import: %s%s",
				len(failedTags),
				strings.Join(reported, "; "),
				suffix,
			),
		)
	}

	return nil
}

func NewAction(opts ...ActionOpts) actions.Fn {
	action := Action{
		labels: map[string]string{},
		namespaceFn: func(_ context.Context, _ *types.ReconciliationRequest) (string, error) {
			return "", nil
		},
	}

	for _, opt := range opts {
		opt(&action)
	}

	return action.run
}
