package helm

import (
	"fmt"
	"time"

	"helm.sh/helm/v4/pkg/strvals"
)

type Option[T any] interface {
	ApplyTo(target *T)
}

type optionFunc[T any] func(*T)

func (f optionFunc[T]) ApplyTo(target *T) {
	f(target)
}

type ClientOption = Option[ClientOptions]
type InstallOption = Option[InstallOptions]
type UninstallOption = Option[UninstallOptions]

type ClientOptions struct {
	RepositoryCache string
	Timeout         time.Duration
	HelmDriver      string
}

func (opts ClientOptions) ApplyTo(target *ClientOptions) {
	if target == nil {
		return
	}
	if opts.RepositoryCache != "" {
		target.RepositoryCache = opts.RepositoryCache
	}
	if opts.Timeout > 0 {
		target.Timeout = opts.Timeout
	}
	if opts.HelmDriver != "" {
		target.HelmDriver = opts.HelmDriver
	}
}

type InstallOptions struct {
	Chart        string
	ChartRepoURL string
	ChartVersion string
	ReleaseName  string
	Namespace    string
	SkipCRDs     bool
	SetValues    []string
	Values       map[string]any
}

func (opts InstallOptions) GetValues() (map[string]any, error) {
	values := cloneMap(opts.Values)
	for _, value := range opts.SetValues {
		if value == "" {
			continue
		}
		if err := strvals.ParseInto(value, values); err != nil {
			return nil, fmt.Errorf("parsing helm set value %q: %w", value, err)
		}
	}

	return values, nil
}

func (opts InstallOptions) ApplyTo(target *InstallOptions) {
	if target == nil {
		return
	}
	if opts.Chart != "" {
		target.Chart = opts.Chart
	}
	if opts.ChartRepoURL != "" {
		target.ChartRepoURL = opts.ChartRepoURL
	}
	if opts.ChartVersion != "" {
		target.ChartVersion = opts.ChartVersion
	}
	if opts.ReleaseName != "" {
		target.ReleaseName = opts.ReleaseName
	}
	if opts.Namespace != "" {
		target.Namespace = opts.Namespace
	}
	if opts.SkipCRDs {
		target.SkipCRDs = true
	}
	if len(opts.SetValues) > 0 {
		target.SetValues = append(target.SetValues, opts.SetValues...)
	}
	if len(opts.Values) > 0 {
		target.Values = mergeMaps(target.Values, opts.Values)
	}
}

type UninstallOptions struct {
	ReleaseName string
	Namespace   string
}

func (opts UninstallOptions) ApplyTo(target *UninstallOptions) {
	if target == nil {
		return
	}
	if opts.ReleaseName != "" {
		target.ReleaseName = opts.ReleaseName
	}
	if opts.Namespace != "" {
		target.Namespace = opts.Namespace
	}
}

func WithRepositoryCache(path string) ClientOption {
	return optionFunc[ClientOptions](func(opts *ClientOptions) {
		if opts == nil || path == "" {
			return
		}
		opts.RepositoryCache = path
	})
}

func WithTimeout(timeout time.Duration) ClientOption {
	return optionFunc[ClientOptions](func(opts *ClientOptions) {
		if opts == nil || timeout <= 0 {
			return
		}
		opts.Timeout = timeout
	})
}

func WithHelmDriver(driver string) ClientOption {
	return optionFunc[ClientOptions](func(opts *ClientOptions) {
		if opts == nil || driver == "" {
			return
		}
		opts.HelmDriver = driver
	})
}

func WithChart(chart string) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || chart == "" {
			return
		}
		opts.Chart = chart
	})
}

func WithChartRepoURL(repoURL string) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || repoURL == "" {
			return
		}
		opts.ChartRepoURL = repoURL
	})
}

func WithChartVersion(version string) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || version == "" {
			return
		}
		opts.ChartVersion = version
	})
}

func WithReleaseName(releaseName string) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || releaseName == "" {
			return
		}
		opts.ReleaseName = releaseName
	})
}

func WithNamespace(namespace string) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || namespace == "" {
			return
		}
		opts.Namespace = namespace
	})
}

func WithValues(values map[string]any) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || len(values) == 0 {
			return
		}
		opts.Values = mergeMaps(opts.Values, values)
	})
}

func WithValue(key string, value string) InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil || key == "" {
			return
		}
		opts.SetValues = append(opts.SetValues, key+"="+value)
	})
}

func WithSkipCRDs() InstallOption {
	return optionFunc[InstallOptions](func(opts *InstallOptions) {
		if opts == nil {
			return
		}
		opts.SkipCRDs = true
	})
}

func WithUninstallReleaseName(releaseName string) UninstallOption {
	return optionFunc[UninstallOptions](func(opts *UninstallOptions) {
		if opts == nil || releaseName == "" {
			return
		}
		opts.ReleaseName = releaseName
	})
}

func WithUninstallNamespace(namespace string) UninstallOption {
	return optionFunc[UninstallOptions](func(opts *UninstallOptions) {
		if opts == nil || namespace == "" {
			return
		}
		opts.Namespace = namespace
	})
}
