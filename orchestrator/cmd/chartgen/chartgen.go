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

package chartgen

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/opendatahub-io/operator-actions-framework/cluster/gvk"
)

const (
	defaultOutputDir   = "config/chart"
	defaultChartName   = "opendatahub-orchestrator"
	defaultChartVer    = "0.1.0"
	templatesDirName   = "templates"
	chartYAMLFilename  = "Chart.yaml"
	helpersTplFilename = "_helpers.tpl"
	valuesYAMLFilename = "values.yaml"
	valuesSchemaFile   = "values.schema.json"
	coreAPIGroup       = "core"
)

// NewCommand returns the cobra command for the chartgen subcommand.
func NewCommand() *cobra.Command {
	var outputDir string
	var chartName string
	var chartVersion string

	cmd := &cobra.Command{
		Use:   "chartgen",
		Short: "Generate a Helm chart from kustomize YAML on stdin",
		Long: `Reads multi-document Kubernetes YAML from stdin (typically piped from
kustomize build) and generates a Helm chart with proper templating.

Example:
  kustomize build config/default | manager chartgen --output config/chart`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(os.Stdin, outputDir, chartName, chartVersion)
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", defaultOutputDir, "Output directory for the chart")
	cmd.Flags().StringVar(&chartName, "name", defaultChartName, "Chart name")
	cmd.Flags().StringVar(&chartVersion, "version", defaultChartVer, "Chart version")

	return cmd
}

func run(
	reader io.Reader,
	outputDir string,
	chartName string,
	chartVersion string,
) error {
	resources, err := decodeResources(reader)
	if err != nil {
		return fmt.Errorf("decoding resources: %w", err)
	}

	groups := groupByGVK(resources)
	values := ExtractDefaults(resources)

	templatesDir := filepath.Join(outputDir, templatesDirName)
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return fmt.Errorf("creating templates directory: %w", err)
	}

	chartFile := filepath.Join(outputDir, chartYAMLFilename)
	if _, err := os.Stat(chartFile); os.IsNotExist(err) {
		if err := writeChartYAML(chartFile, chartName, chartVersion); err != nil {
			return fmt.Errorf("writing %s: %w", chartYAMLFilename, err)
		}
	}

	if err := writeHelpersTpl(filepath.Join(templatesDir, helpersTplFilename)); err != nil {
		return fmt.Errorf("writing %s: %w", helpersTplFilename, err)
	}

	if err := WriteValuesYAML(values, filepath.Join(outputDir, valuesYAMLFilename)); err != nil {
		return fmt.Errorf("writing %s: %w", valuesYAMLFilename, err)
	}

	if err := WriteValuesSchema(filepath.Join(outputDir, valuesSchemaFile)); err != nil {
		return fmt.Errorf("writing %s: %w", valuesSchemaFile, err)
	}

	for resourceGVK, res := range groups {
		filename := gvkToFilename(resourceGVK)
		path := filepath.Join(templatesDir, filename)

		content, err := renderGroup(resourceGVK, res)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", filename, err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Helm chart generated at %s\n", outputDir)

	return nil
}

func decodeResources(reader io.Reader) ([]unstructured.Unstructured, error) {
	var resources []unstructured.Unstructured

	yr := utilyaml.NewYAMLReader(bufio.NewReader(reader))

	for {
		data, err := yr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading YAML document: %w", err)
		}

		data = []byte(strings.TrimSpace(string(data)))
		if len(data) == 0 {
			continue
		}

		var obj unstructured.Unstructured
		if err := yaml.Unmarshal(data, &obj.Object); err != nil {
			return nil, fmt.Errorf("unmarshaling resource: %w", err)
		}

		if obj.Object == nil {
			continue
		}

		resources = append(resources, obj)
	}

	return resources, nil
}

func groupByGVK(resources []unstructured.Unstructured) map[schema.GroupVersionKind][]unstructured.Unstructured {
	groups := make(map[schema.GroupVersionKind][]unstructured.Unstructured)

	for _, r := range resources {
		resourceGVK := r.GroupVersionKind()
		if resourceGVK == gvk.Namespace {
			continue
		}

		groups[resourceGVK] = append(groups[resourceGVK], r)
	}

	return groups
}

func gvkToFilename(resourceGVK schema.GroupVersionKind) string {
	group := strings.ToLower(resourceGVK.Group)
	if group == "" {
		group = coreAPIGroup
	}

	return fmt.Sprintf("%s_%s_%s.yaml",
		group,
		strings.ToLower(resourceGVK.Version),
		strings.ToLower(resourceGVK.Kind),
	)
}
