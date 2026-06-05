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
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"helm.sh/helm/v4/pkg/chart"
	chartloader "helm.sh/helm/v4/pkg/chart/loader"
)

const (
	DefaultTimeout           = 10 * time.Minute
	DefaultConfigHashRollout = true
)

// Module holds the definition of a managed module.
// Optional behavioral interfaces (Configurable, etc.) are
// type-checked on the Ext field.
type Module struct {
	Name              string
	GVK               schema.GroupVersionKind
	Namespace         string // final namespace, computed at registration time
	Runlevel          int
	ChartPath         string
	Timeout           time.Duration
	AdminAcks         []string
	ConfigHashRollout bool
	Values            map[string]any

	// Config returns config values merged into Values.config before chart rendering.
	// Nil means no config injection.
	Config func(ctx context.Context, c client.Client) (map[string]any, error)

	// Ext is type-checked for optional interfaces.
	Ext any

	chartOnce sync.Once
	chart     chart.Charter
	chartErr  error
}

// EffectiveName returns Name if set, otherwise lowercase GVK Kind.
func (m *Module) EffectiveName() string {
	if m.Name != "" {
		return m.Name
	}
	return strings.ToLower(m.GVK.Kind)
}

// Chart returns the Helm chart, lazy-loaded on first call.
func (m *Module) Chart() (chart.Charter, error) {
	m.chartOnce.Do(func() {
		if m.ChartPath == "" {
			m.chartErr = fmt.Errorf("chart path not set for module %q", m.Name)
			return
		}
		m.chart, m.chartErr = chartloader.Load(m.ChartPath)
	})

	return m.chart, m.chartErr
}
