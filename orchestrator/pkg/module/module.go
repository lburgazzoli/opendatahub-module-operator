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

package module

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v4/pkg/chart"
	chartloader "helm.sh/helm/v4/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultTimeout           = 10 * time.Minute
	DefaultConfigHashRollout = true
)

// ModuleSpec is the input used to construct a Module.
type ModuleSpec struct {
	Name              string
	GVK               schema.GroupVersionKind
	Namespace         string
	Runlevel          int
	ChartPath         string
	Timeout           time.Duration
	AdminAcks         []AdminAck
	ConfigHashRollout bool
	Values            map[string]any

	// Config returns config values merged into Values.config before chart rendering.
	// Nil means no config injection.
	Config func(ctx context.Context, c client.Client) (map[string]any, error)

	// Ext is type-checked for optional interfaces.
	Ext any
}

type AdminAck struct {
	Name        string
	Description string
}

type Manifests struct {
	Chart ModuleChart
}

type ModuleChart struct {
	Path       string
	Name       string
	Version    string
	AppVersion string
	Object     chart.Charter
}

// Module holds the definition of a managed module.
// Optional behavioral interfaces (Configurable, etc.) are
// type-checked on the Ext field.
type Module struct {
	Name              string
	GVK               schema.GroupVersionKind
	Namespace         string // final namespace, computed at registration time
	Runlevel          int
	Timeout           time.Duration
	AdminAcks         []AdminAck
	ConfigHashRollout bool
	Values            map[string]any

	// Config returns config values merged into Values.config before chart rendering.
	// Nil means no config injection.
	Config func(ctx context.Context, c client.Client) (map[string]any, error)

	// Ext is type-checked for optional interfaces.
	Ext any

	Manifests Manifests
}

func NewModule(spec ModuleSpec) (*Module, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("module name must be set for GVK %s", spec.GVK.String())
	}
	if spec.ChartPath == "" {
		return nil, fmt.Errorf("chart path not set for module %q", spec.Name)
	}

	moduleChart, err := loadModuleChart(spec.Name, spec.ChartPath)
	if err != nil {
		return nil, err
	}

	return &Module{
		Name:              spec.Name,
		GVK:               spec.GVK,
		Namespace:         spec.Namespace,
		Runlevel:          spec.Runlevel,
		Timeout:           spec.Timeout,
		AdminAcks:         spec.AdminAcks,
		ConfigHashRollout: spec.ConfigHashRollout,
		Values:            spec.Values,
		Config:            spec.Config,
		Ext:               spec.Ext,
		Manifests: Manifests{
			Chart: moduleChart,
		},
	}, nil
}

func loadModuleChart(moduleName string, chartPath string) (ModuleChart, error) {
	chrt, err := chartloader.Load(chartPath)
	if err != nil {
		return ModuleChart{}, fmt.Errorf("loading chart for module %q: %w", moduleName, err)
	}

	info := ModuleChart{
		Path:   chartPath,
		Object: chrt,
	}

	acc, err := chart.NewAccessor(chrt)
	if err != nil {
		return ModuleChart{}, fmt.Errorf("reading chart metadata for module %q: %w", moduleName, err)
	}

	info.Name = acc.Name()

	if md := acc.MetadataAsMap(); md != nil {
		if v, ok := md["version"].(string); ok {
			info.Version = v
		} else if v, ok := md["Version"].(string); ok {
			info.Version = v
		}
		if v, ok := md["appVersion"].(string); ok {
			info.AppVersion = v
		} else if v, ok := md["AppVersion"].(string); ok {
			info.AppVersion = v
		}
	}

	return info, nil
}
