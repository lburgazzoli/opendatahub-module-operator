package platform

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	odhresources "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

type adminAckRequirement struct {
	Name        string
	Description string
	Modules     []string
}

type unsatisfiedAdminAck struct {
	Name        string
	Description string
	Modules     []string
	Value       string
}

func (a *PlatformReconciler) requiredAdminAcks(obj *configApi.Platform) map[string]adminAckRequirement {
	enabled := sets.New(obj.Spec.Modules...)
	requiredModules := map[string]sets.Set[string]{}
	requiredAcks := map[string]module.AdminAck{}

	for _, m := range a.registry.Modules() {
		if !enabled.Has(m.EffectiveName()) {
			continue
		}
		for _, ack := range m.AdminAcks {
			if requiredModules[ack.Name] == nil {
				requiredModules[ack.Name] = sets.New[string]()
			}
			requiredModules[ack.Name].Insert(m.EffectiveName())
			if existing, found := requiredAcks[ack.Name]; !found || existing.Description == "" {
				requiredAcks[ack.Name] = ack
			}
		}
	}

	result := make(map[string]adminAckRequirement, len(requiredModules))
	for ackName, modules := range requiredModules {
		names := modules.UnsortedList()
		slices.Sort(names)
		result[ackName] = adminAckRequirement{
			Name:        ackName,
			Description: requiredAcks[ackName].Description,
			Modules:     names,
		}
	}

	return result
}

func missingAdminAcks(required map[string]adminAckRequirement) []unsatisfiedAdminAck {
	unsatisfied := make([]unsatisfiedAdminAck, 0, len(required))
	for _, ackName := range slices.Sorted(maps.Keys(required)) {
		requiredAck := required[ackName]
		unsatisfied = append(unsatisfied, unsatisfiedAdminAck{
			Name:        requiredAck.Name,
			Description: requiredAck.Description,
			Modules:     requiredAck.Modules,
			Value:       "missing",
		})
	}

	return unsatisfied
}

func unsatisfiedAdminAcks(
	required map[string]adminAckRequirement,
	values map[string]string,
	parseBool func(string) (bool, error),
) []unsatisfiedAdminAck {
	unsatisfied := make([]unsatisfiedAdminAck, 0)
	for _, ackName := range slices.Sorted(maps.Keys(required)) {
		requiredAck := required[ackName]
		raw, found := values[ackName]
		switch {
		case !found:
			unsatisfied = append(unsatisfied, unsatisfiedAdminAck{
				Name:        requiredAck.Name,
				Description: requiredAck.Description,
				Modules:     requiredAck.Modules,
				Value:       "missing",
			})
		default:
			enabled, err := parseBool(raw)
			switch {
			case err != nil, !enabled:
				unsatisfied = append(unsatisfied, unsatisfiedAdminAck{
					Name:        requiredAck.Name,
					Description: requiredAck.Description,
					Modules:     requiredAck.Modules,
					Value:       raw,
				})
			}
		}
	}

	return unsatisfied
}

func adminAcksConditionMessage(namespace string, unsatisfied []unsatisfiedAdminAck) string {
	parts := make([]string, 0, len(unsatisfied))
	for _, ack := range unsatisfied {
		parts = append(parts, formatUnsatisfiedAdminAck(ack))
	}

	return fmt.Sprintf(
		"waiting for admin acks in ConfigMap %s/%s",
		namespace,
		config.AdminAcksConfigMapName,
	) + ": " + strings.Join(parts, "; ")
}

func adminAcksPauseError(namespace string, unsatisfied []unsatisfiedAdminAck) error {
	return actionerrors.NewPauseError(
		adminAckPauseDelay,
		"%s",
		adminAcksConditionMessage(namespace, unsatisfied),
	)
}

func (a *PlatformReconciler) reportModulePruned(obj runtime.Object, name string) {
	if a.recorder == nil {
		return
	}
	a.recorder.Eventf(
		obj,
		nil,
		corev1.EventTypeNormal,
		"ModulePruned",
		"PruneModule",
		"PlatformOperator %q pruned (not in spec.modules)",
		name,
	)
}

func (a *PlatformReconciler) moduleStatus(
	ctx context.Context,
	cli client.Client,
	m *module.Module,
) (configApi.ModuleStatusSummary, error) {
	summary := configApi.ModuleStatusSummary{
		Name:     m.EffectiveName(),
		Runlevel: m.Runlevel,
	}

	po := &configApi.PlatformOperator{}
	po.SetName(m.EffectiveName())

	err := odhresources.Get(ctx, cli, po)

	switch {
	case k8serr.IsNotFound(err):
		return summary, nil
	case err != nil:
		return summary, fmt.Errorf("getting PlatformOperator %q: %w", m.EffectiveName(), err)
	default:
		summary.Distribution = po.Status.Distribution.Current
	}

	return summary, nil
}

func (a *PlatformReconciler) reportUnsatisfiedAdminAcks(obj runtime.Object, unsatisfied []unsatisfiedAdminAck) {
	if a.recorder == nil {
		return
	}
	for _, ack := range unsatisfied {
		a.recorder.Eventf(
			obj,
			nil,
			corev1.EventTypeWarning,
			"AdminAckRequired",
			"WaitForAdminAck",
			"Admin ack %q required: %s",
			ack.Name,
			formatUnsatisfiedAdminAck(ack),
		)
	}
}

func formatUnsatisfiedAdminAck(ack unsatisfiedAdminAck) string {
	message := fmt.Sprintf(
		"%s (modules: %s",
		ack.Name,
		strings.Join(ack.Modules, ","),
	)
	if ack.Description != "" {
		message += fmt.Sprintf(", description: %s", ack.Description)
	}
	message += fmt.Sprintf(", value: %q)", ack.Value)

	return message
}

func (a *PlatformReconciler) runlevelComplete(
	ctx context.Context,
	rr *types.ReconciliationRequest,
	obj *configApi.Platform,
	level int,
) (bool, error) {
	upgradeInProgress := obj.Status.Distribution.Current.Version != "" &&
		obj.Status.Distribution.Current.Version != obj.Status.Distribution.Target.Version

	for _, m := range a.enabledModulesAtRunlevel(obj, level) {
		po := &configApi.PlatformOperator{}
		po.SetName(m.EffectiveName())

		err := odhresources.Get(ctx, rr.Client, po)

		switch {
		case k8serr.IsNotFound(err):
			return false, nil
		case err != nil:
			return false, fmt.Errorf("getting PlatformOperator %q: %w", m.EffectiveName(), err)
		case upgradeInProgress && po.Status.Distribution.Current.Version != obj.Status.Distribution.Target.Version:
			return false, nil
		}
	}

	return true, nil
}

func (a *PlatformReconciler) enabledModulesAtRunlevel(
	obj *configApi.Platform,
	level int,
) []*module.Module {
	enabled := sets.New(obj.Spec.Modules...)
	modules := make([]*module.Module, 0)

	for _, m := range a.registry.ModulesAtRunlevel(level) {
		if enabled.Has(m.EffectiveName()) {
			modules = append(modules, m)
		}
	}

	return modules
}
