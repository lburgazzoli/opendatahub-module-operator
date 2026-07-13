package helm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	helmchart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"

	"github.com/k8s-manifest-kit/renderer-helm/pkg/locator"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

func loadChart(
	ctx context.Context,
	opts InstallOptions,
	repositoryCache string,
) (*helmchart.Chart, error) {
	chartRef, err := resolveChartReference(opts.Chart, opts.ChartRepoURL)
	if err != nil {
		return nil, err
	}

	result, err := locator.Locate(ctx, &locator.Request{
		Name:            chartRef,
		RepoURL:         opts.ChartRepoURL,
		Version:         opts.ChartVersion,
		RepositoryCache: repositoryCache,
	})
	if err != nil {
		return nil, fmt.Errorf("locating chart %q: %w", opts.Chart, err)
	}

	chrt, err := loader.Load(result.Path)
	if err != nil {
		return nil, fmt.Errorf("loading chart from %q: %w", result.Path, err)
	}

	return chrt, nil
}

func resolveChartReference(chart string, repoURL string) (string, error) {
	if chart == "" {
		return "", fmt.Errorf("chart is empty")
	}

	if repoURL != "" || strings.HasPrefix(chart, "oci://") || filepath.IsAbs(chart) || strings.HasPrefix(chart, ".") {
		return chart, nil
	}

	if _, err := os.Stat(chart); err == nil {
		return chart, nil
	}

	moduleChart, err := support.ModulePath(chart)
	if err != nil {
		return "", fmt.Errorf("resolving module chart path for %q: %w", chart, err)
	}
	if _, err := os.Stat(moduleChart); err == nil {
		return moduleChart, nil
	}

	return chart, nil
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}

	return mergeMaps(nil, src)
}

func mergeMaps(dst map[string]any, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for key, value := range src {
		nested, ok := value.(map[string]any)
		if !ok {
			dst[key] = value
			continue
		}

		existing, ok := dst[key].(map[string]any)
		if !ok {
			existing = map[string]any{}
		}
		dst[key] = mergeMaps(existing, nested)
	}

	return dst
}
