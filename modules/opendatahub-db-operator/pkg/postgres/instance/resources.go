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

package instance

import (
	"context"
	"fmt"
	"text/template"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/assets"
)

func Resources(
	ctx context.Context,
	data Data,
) ([]unstructured.Unstructured, error) {
	sources := []gotemplate.Source{{
		FS:     assets.Manifests,
		Path:   "manifests/embedded/*.yaml.tmpl",
		Values: gotemplate.Values(Values(data)),
	}}

	renderer, err := gotemplate.New(
		sources,
		gotemplate.WithFuncs(template.FuncMap{
			"toYaml":  gotemplate.ToYAML,
			"indent":  gotemplate.Indent,
			"nindent": gotemplate.Nindent,
		}),
		gotemplate.WithContentHash(false),
	)
	if err != nil {
		return nil, fmt.Errorf("creating instance renderer: %w", err)
	}

	rendered, err := renderer.Process(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("rendering instance resources: %w", err)
	}

	return rendered, nil
}
