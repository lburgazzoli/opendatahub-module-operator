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

// Package v1alpha1 contains API Schema definitions for the services v1alpha1
// API group: DatabaseService, the module-enablement CR (docs/plan.md §4).
// A distinct group from components.platform.opendatahub.io -- this CR
// represents a platform infrastructure service, not a user-facing ML/serving
// component the way Ray/ModelRegistry's Module CRs do.
// +kubebuilder:object:generate=true
// +groupName=services.platform.opendatahub.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	GroupName = "services.platform.opendatahub.io"
	Version   = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects.
	// This name is used by applyconfiguration generators (e.g. controller-gen).
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

	// GroupVersion is an alias for SchemeGroupVersion, for backward compatibility.
	GroupVersion = SchemeGroupVersion

	// SchemeBuilder collects each type file's registration function. Built
	// directly on apimachinery's runtime.SchemeBuilder rather than
	// controller-runtime's scheme.Builder helper, which is deprecated for
	// api packages precisely because it drags in controller-runtime as a
	// dependency of a package that should only depend on the standard
	// library, apimachinery, and other api packages.
	SchemeBuilder = &runtime.SchemeBuilder{}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		metav1.AddToGroupVersion(s, SchemeGroupVersion)
		return nil
	})
}
