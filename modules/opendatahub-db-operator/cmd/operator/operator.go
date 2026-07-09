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

package operator

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	modulemgr "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
)

func NewCommand() *cobra.Command {
	v := moduleconfig.NewViper()
	var registerErr error

	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Start the module operator",
		RunE: func(cmd *cobra.Command, args []string) error {
			if registerErr != nil {
				return registerErr
			}

			return run(cmd, v)
		},
	}

	registerErr = moduleconfig.RegisterFlags(cmd, v)

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	cfg, err := moduleconfig.LoadFromViper(v)
	if err != nil {
		return fmt.Errorf("loading operator config: %w", err)
	}

	logger, err := cfg.Controller.Zap.NewLogger()
	if err != nil {
		return fmt.Errorf("building zap logger: %w", err)
	}
	ctrl.SetLogger(logger)

	mgr, err := modulemgr.New(cmd.Context(), ctrl.GetConfigOrDie(), cfg)
	if err != nil {
		return err
	}

	return mgr.Start(cmd.Context())
}
