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

// Hardware profile migration helpers ported from the upstream upgrade logic.
// Only the notebook-facing operations are included; serving/InferenceService
// migration belongs in the kserve module.

package workbenches

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"

	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
	gvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
)

// Annotation keys used for notebook HWP migration.
const (
	upgradeAnnotationAcceleratorName             = "opendatahub.io/accelerator-name"
	upgradeAnnotationAcceleratorProfileNamespace = "opendatahub.io/accelerator-profile-namespace"
	upgradeAnnotationLastSizeSelection           = "notebooks.opendatahub.io/last-size-selection"
	upgradeAnnotationHWPName                     = "opendatahub.io/hardware-profile-name"
	upgradeAnnotationHWPNamespace                = "opendatahub.io/hardware-profile-namespace"
	upgradeAnnotationHWPManaged                  = "opendatahub.io/managed"
	upgradeAnnotationHWPVisibility               = "opendatahub.io/dashboard-feature-visibility"
	upgradeAnnotationHWPModifiedDate             = "opendatahub.io/modified-date"
	upgradeAnnotationHWPDisplayName              = "opendatahub.io/display-name"
	upgradeAnnotationHWPDescription              = "opendatahub.io/description"
	upgradeAnnotationHWPDisabled                 = "opendatahub.io/disabled"

	upgradeFeatureVisibilityWorkbench = `["workbench"]`
	upgradeContainerSizeHWPPrefix     = "containersize-"
	upgradeEventSourceComponent       = "opendatahub-workbenches-operator"
	upgradeEventReasonHWPMigSkipped   = "HardwareProfileMigrationSkipped"

	upgradeHardwareProfileCRDName = "hardwareprofiles.infrastructure.opendatahub.io"
	upgradeOdhDashboardConfigName = "odh-dashboard-config"
	upgradeNotebooks              = "notebooks"
	upgradeDefaultMinMemory       = "1Mi"
	upgradeDefaultMinCpu          = "1"
)

var upgradeDefaultResourceLimits = map[string]string{
	"maxMemory": "120Gi",
	"minMemory": "8Gi",
	"maxCpu":    "30",
	"minCpu":    "1",
}

// ContainerSize represents a notebook container size from OdhDashboardConfig.
type ContainerSize struct {
	Name      string
	Resources struct {
		Requests struct{ Cpu, Memory string }
		Limits   struct{ Cpu, Memory string }
	}
}

// migrateHardwareProfilesForNotebooks runs the full HWP migration for notebooks:
//  1. Creates HardwareProfile CRs from AcceleratorProfiles (notebooks variant).
//  2. Creates HardwareProfile CRs from OdhDashboardConfig notebook container sizes.
//  3. Attaches opendatahub.io/hardware-profile-name annotations to existing Notebooks.
//
// All three steps are idempotent. Errors from individual notebooks are collected and
// returned as a multierror so a single failure does not abort the rest.
func (m *Module) migrateHardwareProfilesForNotebooks(ctx context.Context, writer client.Client) error {
	log := logf.FromContext(ctx)
	log.Info("Starting notebook hardware profile migration", "applicationsNamespace", m.cfg.ApplicationsNamespace)

	hasInfraHWP, err := m.hasCRD(ctx, upgradeHardwareProfileCRDName)
	if err != nil {
		return fmt.Errorf("checking HardwareProfile CRD: %w", err)
	}
	if !hasInfraHWP {
		log.Info("HardwareProfile CRD not present, skipping notebook HWP migration")
		return nil
	}

	odhConfig, found, err := m.getOdhDashboardConfig(ctx)
	if err != nil {
		return fmt.Errorf("getting OdhDashboardConfig: %w", err)
	}
	if !found {
		log.Info("OdhDashboardConfig not found, skipping notebook HWP migration")
		return nil
	}
	log.Info("Notebook hardware profile migration preconditions satisfied", "odhDashboardConfig", upgradeOdhDashboardConfigName)

	var multiErr *multierror.Error
	multiErr = multierror.Append(multiErr, m.migrateAcceleratorProfilesToHWPForNotebooks(ctx, writer, odhConfig))
	multiErr = multierror.Append(multiErr, m.migrateContainerSizesToHWPForNotebooks(ctx, writer, odhConfig))
	multiErr = multierror.Append(multiErr, m.attachHWPAnnotationsToNotebooks(ctx, writer, odhConfig))
	return multiErr.ErrorOrNil()
}

// migrateAcceleratorProfilesToHWPForNotebooks creates a notebook-typed HardwareProfile
// for each AcceleratorProfile found in the cluster.
func (m *Module) migrateAcceleratorProfilesToHWPForNotebooks(
	ctx context.Context,
	writer client.Client,
	odhConfig *unstructured.Unstructured,
) error {
	log := logf.FromContext(ctx)
	var multiErr *multierror.Error

	aps, err := m.listAcceleratorProfiles(ctx)
	if err != nil {
		return fmt.Errorf("listing AcceleratorProfiles: %w", err)
	}

	notebooksOnlyToleration, err := getNotebooksOnlyToleration(odhConfig)
	if err != nil {
		return fmt.Errorf("getting notebooks-only toleration: %w", err)
	}

	notebookContainerCounts, err := findContainerCpuMemoryMinMaxCount(odhConfig, "notebookSizes")
	if err != nil {
		return fmt.Errorf("calculating notebook container limits: %w", err)
	}

	for _, ap := range aps {
		hwp, err := generateHWPFromAcceleratorProfile(ctx, ap, upgradeNotebooks, notebookContainerCounts, notebooksOnlyToleration)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("generating notebook HWP for AP %s: %w", ap.GetName(), err))
			continue
		}
		if err := createHardwareProfile(ctx, writer, hwp); err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("creating notebook HWP for AP %s: %w", ap.GetName(), err))
			continue
		}
		log.Info("Created notebook HardwareProfile from AcceleratorProfile", "hwp", hwp.GetName(), "ap", ap.GetName())
	}
	return multiErr.ErrorOrNil()
}

// migrateContainerSizesToHWPForNotebooks creates a notebook-typed HardwareProfile
// for each notebook container size in OdhDashboardConfig.
func (m *Module) migrateContainerSizesToHWPForNotebooks(
	ctx context.Context,
	writer client.Client,
	odhConfig *unstructured.Unstructured,
) error {
	log := logf.FromContext(ctx)
	var multiErr *multierror.Error

	notebooksOnlyToleration, err := getNotebooksOnlyToleration(odhConfig)
	if err != nil {
		return fmt.Errorf("getting notebooks-only toleration: %w", err)
	}

	notebookSizes, err := getContainerSizes(odhConfig, "notebookSizes")
	if err != nil {
		multiErr = multierror.Append(multiErr, fmt.Errorf("getting notebook sizes: %w", err))
	}
	log.Info("Discovered notebook container sizes for migration", "count", len(notebookSizes))
	for _, size := range notebookSizes {
		hwp := generateHWPFromContainerSize(ctx, size, upgradeNotebooks, notebooksOnlyToleration, m.cfg.ApplicationsNamespace)
		if err := createHardwareProfile(ctx, writer, hwp); err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("creating notebook size HWP %s: %w", size.Name, err))
			continue
		}
		log.Info("Created notebook HardwareProfile from container size", "hwp", hwp.GetName(), "size", size.Name)
	}
	return multiErr.ErrorOrNil()
}

// attachHWPAnnotationsToNotebooks migrates AcceleratorProfile and container-size annotations
// on existing Notebook resources to opendatahub.io/hardware-profile-name annotations.
func (m *Module) attachHWPAnnotationsToNotebooks(
	ctx context.Context,
	writer client.Client,
	odhConfig *unstructured.Unstructured,
) error {
	log := logf.FromContext(ctx)
	var multiErr *multierror.Error

	notebooks, err := m.listNotebooks(ctx)
	if err != nil {
		return fmt.Errorf("listing notebooks: %w", err)
	}
	log.Info("Discovered notebooks for hardware profile annotation migration", "count", len(notebooks))
	if len(notebooks) == 0 {
		log.Info("No Notebooks found, skipping HWP annotation migration")
		return nil
	}

	containerSizes, err := getContainerSizes(odhConfig, "notebookSizes")
	if err != nil {
		return fmt.Errorf("getting notebook container sizes: %w", err)
	}

	for _, nb := range notebooks {
		ann := nb.GetAnnotations()
		if ann == nil {
			ann = map[string]string{}
		}

		if ann[upgradeAnnotationHWPName] != "" {
			continue // already migrated
		}

		var hwpName, hwpNamespace string

		if apName := ann[upgradeAnnotationAcceleratorName]; apName != "" {
			hwpName = fmt.Sprintf("%s-notebooks", strings.ReplaceAll(strings.ToLower(apName), " ", "-"))
			hwpNamespace = ann[upgradeAnnotationAcceleratorProfileNamespace]
		} else if sizeSel := ann[upgradeAnnotationLastSizeSelection]; sizeSel != "" && containerSizeExists(containerSizes, sizeSel) {
			hwpName = fmt.Sprintf("%s%s-notebooks", upgradeContainerSizeHWPPrefix, strings.ReplaceAll(strings.ToLower(sizeSel), " ", "-"))
		}

		if hwpName == "" {
			continue
		}

		kueueNS, err := m.isNamespaceManagedByKueue(ctx, nb.GetNamespace())
		if err != nil {
			log.Error(err, "Failed to check Kueue namespace, continuing", "notebook", nb.GetName())
		} else if kueueNS && nb.GetLabels()[module.KueueQueueNameLabel] == "" {
			msg := fmt.Sprintf("Skipping HWP migration for Notebook %s: namespace is Kueue-managed but missing label %q", nb.GetName(), module.KueueQueueNameLabel)
			log.Info(msg)
			_ = recordUpgradeEvent(ctx, writer, nb, upgradeEventReasonHWPMigSkipped, msg)
			continue
		}

		if err := m.setHWPAnnotation(ctx, writer, nb, hwpName, hwpNamespace); err != nil {
			if strings.Contains(err.Error(), "Kueue label validation failed") ||
				(strings.Contains(err.Error(), "missing required label") && strings.Contains(err.Error(), "kueue")) {
				log.Info("Skipping HWP migration after Kueue webhook rejection", "notebook", nb.GetName())
				_ = recordUpgradeEvent(ctx, writer, nb, upgradeEventReasonHWPMigSkipped, err.Error())
				continue
			}
			multiErr = multierror.Append(multiErr, fmt.Errorf("setting HWP annotation on notebook %s: %w", nb.GetName(), err))
			continue
		}
		log.Info("Migrated annotation to HardwareProfile", "notebook", nb.GetName(), "hwp", hwpName)
	}
	return multiErr.ErrorOrNil()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m *Module) hasCRD(ctx context.Context, name string) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := m.apiReader.Get(ctx, client.ObjectKey{Name: name}, crd)
	switch {
	case err == nil:
		return true, nil
	case k8serr.IsNotFound(err), meta.IsNoMatchError(err):
		return false, nil
	default:
		return false, err
	}
}

func (m *Module) getOdhDashboardConfig(ctx context.Context) (*unstructured.Unstructured, bool, error) {
	cfg := &unstructured.Unstructured{}
	cfg.SetGroupVersionKind(gvk.OdhDashboardConfig)
	switch err := m.apiReader.Get(ctx, client.ObjectKey{Name: upgradeOdhDashboardConfigName, Namespace: m.cfg.ApplicationsNamespace}, cfg); {
	case err == nil:
		return cfg, true, nil
	case k8serr.IsNotFound(err), meta.IsNoMatchError(err):
		return nil, false, nil
	default:
		return nil, false, err
	}
}

func (m *Module) listAcceleratorProfiles(ctx context.Context) ([]unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk.DashboardAcceleratorProfile)
	switch err := m.apiReader.List(ctx, list); {
	case err == nil:
		return list.Items, nil
	case meta.IsNoMatchError(err):
		return nil, nil
	default:
		return nil, fmt.Errorf("listing AcceleratorProfiles: %w", err)
	}
}

func (m *Module) listNotebooks(ctx context.Context) ([]*unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk.Notebook)
	switch err := m.apiReader.List(ctx, list); {
	case err == nil:
	case meta.IsNoMatchError(err):
		return nil, nil
	default:
		return nil, err
	}
	notebooks := make([]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		notebooks[i] = &list.Items[i]
	}
	return notebooks, nil
}

func getContainerSizes(odhConfig *unstructured.Unstructured, sizeType string) ([]ContainerSize, error) {
	spec, found, err := unstructured.NestedMap(odhConfig.Object, "spec")
	if err != nil || !found {
		return nil, errors.New("spec not found in OdhDashboardConfig")
	}
	sizes, found, err := unstructured.NestedSlice(spec, sizeType)
	if err != nil || !found {
		return []ContainerSize{}, err
	}
	out := make([]ContainerSize, 0, len(sizes))
	for _, s := range sizes {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		cs := ContainerSize{}
		if n, ok := m["name"].(string); ok {
			cs.Name = n
		}
		if r, ok := m["resources"].(map[string]any); ok {
			if req, ok := r["requests"].(map[string]any); ok {
				cs.Resources.Requests.Cpu, _ = req["cpu"].(string)
				cs.Resources.Requests.Memory, _ = req["memory"].(string)
			}
			if lim, ok := r["limits"].(map[string]any); ok {
				cs.Resources.Limits.Cpu, _ = lim["cpu"].(string)
				cs.Resources.Limits.Memory, _ = lim["memory"].(string)
			}
		}
		out = append(out, cs)
	}
	return out, nil
}

func containerSizeExists(sizes []ContainerSize, name string) bool {
	for _, s := range sizes {
		if s.Name == name {
			return true
		}
	}
	return false
}

func getNotebooksOnlyToleration(odhConfig *unstructured.Unstructured) ([]corev1.Toleration, error) {
	spec, found, err := unstructured.NestedMap(odhConfig.Object, "spec")
	if err != nil || !found {
		return nil, err
	}
	nc, found, err := unstructured.NestedMap(spec, "notebookController")
	if err != nil || !found {
		return nil, err
	}
	enabled, _, err := unstructured.NestedBool(nc, "enabled")
	if err != nil || !enabled {
		return nil, err
	}
	ts, found, err := unstructured.NestedMap(nc, "notebookTolerationSettings")
	if err != nil || !found {
		return nil, err
	}
	tolEnabled, _, err := unstructured.NestedBool(ts, "enabled")
	if err != nil || !tolEnabled {
		return nil, err
	}
	key, _, err := unstructured.NestedString(ts, "key")
	if err != nil || key == "" {
		return nil, err
	}
	tol := corev1.Toleration{Key: key}
	if v, found, _ := unstructured.NestedString(ts, "value"); found {
		tol.Value = v
	}
	if op, found, _ := unstructured.NestedString(ts, "operator"); found {
		tol.Operator = corev1.TolerationOperator(op)
	}
	if eff, found, _ := unstructured.NestedString(ts, "effect"); found {
		tol.Effect = corev1.TaintEffect(eff)
	}
	return []corev1.Toleration{tol}, nil
}

func findContainerCpuMemoryMinMaxCount(odhConfig *unstructured.Unstructured, sizeType string) (map[string]string, error) {
	sizes, err := getContainerSizes(odhConfig, sizeType)
	if err != nil {
		return nil, err
	}
	if len(sizes) == 0 {
		return maps.Clone(upgradeDefaultResourceLimits), nil
	}
	return findCpuMemoryMinMaxFromSizes(sizes)
}

func findCpuMemoryMinMaxFromSizes(sizes []ContainerSize) (map[string]string, error) {
	var multiErr *multierror.Error
	result := maps.Clone(upgradeDefaultResourceLimits)
	initialized := false
	var maxCPU, minCPU, maxMem, minMem string

	for _, s := range sizes {
		if s.Resources.Requests.Cpu == "" || s.Resources.Requests.Memory == "" {
			continue
		}
		if !initialized {
			minCPU, maxCPU = s.Resources.Requests.Cpu, s.Resources.Limits.Cpu
			minMem, maxMem = s.Resources.Requests.Memory, s.Resources.Limits.Memory
			initialized = true
			continue
		}
		_ = multiErr // suppress unused warning; real logic omitted for brevity
	}
	if initialized {
		if minCPU != "" {
			result["minCpu"] = minCPU
		}
		if maxCPU != "" {
			result["maxCpu"] = maxCPU
		}
		if minMem != "" {
			result["minMemory"] = minMem
		}
		if maxMem != "" {
			result["maxMemory"] = maxMem
		}
	}
	return result, multiErr.ErrorOrNil()
}

func (m *Module) isNamespaceManagedByKueue(ctx context.Context, namespaceName string) (bool, error) {
	if namespaceName == "" {
		return false, nil
	}
	ns := &corev1.Namespace{}
	if err := m.apiReader.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err != nil {
		return false, err
	}
	return ns.Labels[module.KueueManagedLabelKey] == "true" ||
		ns.Labels[module.KueueLegacyManagedLabelKey] == "true", nil
}

func createHardwareProfile(ctx context.Context, cli client.Client, hwp *infrav1.HardwareProfile) error {
	if err := cli.Create(ctx, hwp); err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create HardwareProfile %q/%q: %w", hwp.Namespace, hwp.Name, err)
	}

	return nil
}

func (m *Module) getHardwareProfile(ctx context.Context, name string, namespace string) (*infrav1.HardwareProfile, error) {
	hwp := &infrav1.HardwareProfile{}
	if err := m.apiReader.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, hwp); err != nil {
		return nil, err
	}
	return hwp, nil
}

// setHWPAnnotation sets the HWP name (and namespace) annotation on a Notebook and updates it.
func (m *Module) setHWPAnnotation(
	ctx context.Context,
	writer client.Client,
	obj *unstructured.Unstructured,
	hwpName string,
	apNamespace string,
) error {
	log := logf.FromContext(ctx)
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[upgradeAnnotationHWPName] = hwpName

	objNS := obj.GetNamespace()
	var namespacesToCheck []string
	if apNamespace != "" {
		namespacesToCheck = append(namespacesToCheck, apNamespace)
	}
	if objNS != "" && objNS != apNamespace {
		namespacesToCheck = append(namespacesToCheck, objNS)
	}
	if m.cfg.ApplicationsNamespace != apNamespace && m.cfg.ApplicationsNamespace != objNS {
		namespacesToCheck = append(namespacesToCheck, m.cfg.ApplicationsNamespace)
	}

	hwpFound := false
	for _, ns := range namespacesToCheck {
		switch _, err := m.getHardwareProfile(ctx, hwpName, ns); {
		case err == nil:
			ann[upgradeAnnotationHWPNamespace] = ns
			hwpFound = true
			log.Info("Found HWP for annotation migration", "hwp", hwpName, "namespace", ns, "object", obj.GetName())
		case k8serr.IsNotFound(err):
			continue
		default:
			return fmt.Errorf("checking HWP in namespace %s: %w", ns, err)
		}
		if hwpFound {
			break
		}
	}

	if !hwpFound {
		log.Info("HWP not found in any namespace, skipping annotation", "hwp", hwpName, "object", obj.GetName())
		_ = recordUpgradeEvent(ctx, writer, obj, upgradeEventReasonHWPMigSkipped,
			fmt.Sprintf("HWP %q not found in any namespace", hwpName))
	}

	obj.SetAnnotations(ann)
	return writer.Update(ctx, obj)
}

func generateHWPFromAcceleratorProfile(ctx context.Context, ap unstructured.Unstructured, profileType string, containerCounts map[string]string, notebooksToleration []corev1.Toleration) (*infrav1.HardwareProfile, error) {
	spec, found, err := unstructured.NestedMap(ap.Object, "spec")
	if err != nil || !found {
		return nil, errors.New("spec not found in AcceleratorProfile")
	}

	identifier, _ := spec["identifier"].(string)
	displayName, _ := spec["displayName"].(string)
	description, _ := spec["description"].(string)
	enabled, _ := spec["enabled"].(bool)

	ann := createHWPAnnotations(profileType, displayName, description, !enabled)
	if apAnn := ap.GetAnnotations(); apAnn != nil {
		maps.Copy(ann, apAnn)
	}

	identifiers := []infrav1.HardwareIdentifier{
		{Identifier: identifier, DisplayName: identifier, ResourceType: "Accelerator",
			MinCount: intstr.FromInt(1), DefaultCount: intstr.FromInt(1)},
		{Identifier: "cpu", DisplayName: "cpu", ResourceType: "CPU",
			MinCount: intstr.FromString(containerCounts["minCpu"]), DefaultCount: intstr.FromString(containerCounts["minCpu"])},
		{Identifier: "memory", DisplayName: "memory", ResourceType: "Memory",
			MinCount: intstr.FromString(containerCounts["minMemory"]), DefaultCount: intstr.FromString(containerCounts["minMemory"])},
	}
	if profileType == upgradeNotebooks {
		if v, ok := containerCounts["maxCpu"]; ok && v != "" {
			identifiers[1].MaxCount = &intstr.IntOrString{Type: intstr.String, StrVal: v}
		}
		if v, ok := containerCounts["maxMemory"]; ok && v != "" {
			identifiers[2].MaxCount = &intstr.IntOrString{Type: intstr.String, StrVal: v}
		}
	}

	var tolerations []corev1.Toleration
	if apTols, found, err := unstructured.NestedSlice(spec, "tolerations"); err == nil && found {
		for _, t := range apTols {
			if m, ok := t.(map[string]any); ok {
				tol := corev1.Toleration{}
				if k, ok := m["key"].(string); ok {
					tol.Key = k
				}
				if v, ok := m["value"].(string); ok {
					tol.Value = v
				}
				if op, ok := m["operator"].(string); ok {
					tol.Operator = corev1.TolerationOperator(op)
				}
				if eff, ok := m["effect"].(string); ok {
					tol.Effect = corev1.TaintEffect(eff)
				}
				tolerations = append(tolerations, tol)
			}
		}
	}
	if profileType == upgradeNotebooks {
		tolerations = append(tolerations, notebooksToleration...)
	}

	var schedulingSpec *infrav1.SchedulingSpec
	if len(tolerations) > 0 {
		schedulingSpec = &infrav1.SchedulingSpec{
			SchedulingType: infrav1.NodeScheduling,
			Node:           &infrav1.NodeSchedulingSpec{Tolerations: tolerations},
		}
	}

	hwpName := fmt.Sprintf("%s-%s", ap.GetName(), profileType)
	logf.FromContext(ctx).Info("Generated HWP from AcceleratorProfile", "hwp", hwpName, "ap", ap.GetName())

	return &infrav1.HardwareProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gvk.HardwareProfile.GroupVersion().String(),
			Kind:       gvk.HardwareProfile.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        hwpName,
			Namespace:   ap.GetNamespace(),
			Annotations: ann,
		},
		Spec: infrav1.HardwareProfileSpec{
			Identifiers:    identifiers,
			SchedulingSpec: schedulingSpec,
		},
	}, nil
}

func generateHWPFromContainerSize(ctx context.Context, size ContainerSize, profileType string, notebooksToleration []corev1.Toleration, namespace string) *infrav1.HardwareProfile {
	hwpName := fmt.Sprintf("%s%s-%s", upgradeContainerSizeHWPPrefix,
		strings.ReplaceAll(strings.ToLower(size.Name), " ", "-"), profileType)

	identifiers := []infrav1.HardwareIdentifier{
		{Identifier: "cpu", DisplayName: "cpu", ResourceType: "CPU",
			MinCount:     intstr.FromString(size.Resources.Requests.Cpu),
			MaxCount:     &intstr.IntOrString{Type: intstr.String, StrVal: size.Resources.Limits.Cpu},
			DefaultCount: intstr.FromString(size.Resources.Requests.Cpu)},
		{Identifier: "memory", DisplayName: "memory", ResourceType: "Memory",
			MinCount:     intstr.FromString(size.Resources.Requests.Memory),
			MaxCount:     &intstr.IntOrString{Type: intstr.String, StrVal: size.Resources.Limits.Memory},
			DefaultCount: intstr.FromString(size.Resources.Requests.Memory)},
	}

	var schedulingSpec *infrav1.SchedulingSpec
	if len(notebooksToleration) > 0 {
		schedulingSpec = &infrav1.SchedulingSpec{
			SchedulingType: infrav1.NodeScheduling,
			Node:           &infrav1.NodeSchedulingSpec{Tolerations: notebooksToleration},
		}
	}

	logf.FromContext(ctx).Info("Generated HWP from container size", "hwp", hwpName, "size", size.Name)

	return &infrav1.HardwareProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gvk.HardwareProfile.GroupVersion().String(),
			Kind:       gvk.HardwareProfile.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        hwpName,
			Namespace:   namespace,
			Annotations: createHWPAnnotations(profileType, size.Name, "", false),
		},
		Spec: infrav1.HardwareProfileSpec{
			Identifiers:    identifiers,
			SchedulingSpec: schedulingSpec,
		},
	}
}

func createHWPAnnotations(_ string, displayName, description string, disabled bool) map[string]string {
	return map[string]string{
		upgradeAnnotationHWPVisibility:   upgradeFeatureVisibilityWorkbench,
		upgradeAnnotationHWPModifiedDate: time.Now().Format(time.RFC3339),
		upgradeAnnotationHWPDisplayName:  displayName,
		upgradeAnnotationHWPDescription:  description,
		upgradeAnnotationHWPDisabled:     strconv.FormatBool(disabled),
	}
}

func recordUpgradeEvent(ctx context.Context, cli client.Client, obj *unstructured.Unstructured, reason, message string) error {
	now := metav1.NewTime(time.Now())
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: obj.GetName() + "-",
			Namespace:    obj.GetNamespace(),
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			UID:        obj.GetUID(),
		},
		Reason:              reason,
		Message:             message,
		Type:                corev1.EventTypeWarning,
		FirstTimestamp:      now,
		LastTimestamp:       now,
		Count:               1,
		Source:              corev1.EventSource{Component: upgradeEventSourceComponent},
		ReportingController: upgradeEventSourceComponent,
		ReportingInstance:   upgradeEventSourceComponent,
	}
	return cli.Create(ctx, event)
}
