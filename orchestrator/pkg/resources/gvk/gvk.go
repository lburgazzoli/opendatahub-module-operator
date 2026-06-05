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

package gvk

import "k8s.io/apimachinery/pkg/runtime/schema"

var ComponentsGV = schema.GroupVersion{
	Group:   "components.platform.opendatahub.io",
	Version: "v1alpha1",
}

var (
	Ray                  = ComponentsGV.WithKind("Ray")
	Spark                = ComponentsGV.WithKind("Spark")
	Feast                = ComponentsGV.WithKind("Feast")
	Ogx                  = ComponentsGV.WithKind("Ogx")
	MLflow               = ComponentsGV.WithKind("MLflow")
	TrustyAI             = ComponentsGV.WithKind("TrustyAI")
	Trainer              = ComponentsGV.WithKind("Trainer")
	ModelRegistry        = ComponentsGV.WithKind("ModelRegistry")
	DataSciencePipelines = ComponentsGV.WithKind("DataSciencePipelines")
	Workbenches          = ComponentsGV.WithKind("Workbenches")
)
