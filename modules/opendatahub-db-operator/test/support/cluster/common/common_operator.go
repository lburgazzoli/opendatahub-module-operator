package common

import (
	"context"
	"fmt"

	supporthelm "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/helm"
	supportlogger "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/logger"
)

const (
	operatorReleaseName      = "odh-db-operator"
	operatorLogPrefix        = "[operator logs] "
	operatorLogContainerName = "manager"
)

var operatorLogSelector = map[string]string{
	"app.kubernetes.io/name": "opendatahub-db-operator",
}

func (b *Base) InstallOperator(ctx context.Context) error {
	if b == nil || b.testCfg == nil {
		return fmt.Errorf("test config is nil")
	}
	testCfg := b.testCfg
	if testCfg.Operator.Image == "" {
		return fmt.Errorf("operator image is empty")
	}

	helmClient, err := supporthelm.New(b.cfg)
	if err != nil {
		return fmt.Errorf("creating helm client: %w", err)
	}
	if err := helmClient.Uninstall(
		supporthelm.WithUninstallNamespace(testCfg.Operator.Namespace),
		supporthelm.WithUninstallReleaseName(operatorReleaseName),
	); err != nil {
		return fmt.Errorf("resetting helm release: %w", err)
	}
	if err := helmClient.Install(ctx,
		supporthelm.WithChart("config/chart"),
		supporthelm.WithReleaseName(operatorReleaseName),
		supporthelm.WithNamespace(testCfg.Operator.Namespace),
		supporthelm.WithSkipCRDs(),
		supporthelm.WithValue("operator.image.ref", testCfg.Operator.Image),
		supporthelm.WithValue("platform.type", testCfg.Operator.PlatformType),
		supporthelm.WithValue("platform.version", testCfg.Operator.PlatformVersion),
	); err != nil {
		return err
	}

	if testCfg.Operator.Logs {
		operatorLogger, err := supportlogger.New(
			b.cfg,
			supportlogger.WithDefaultNamespace(testCfg.Operator.Namespace),
			supportlogger.WithDefaultPrefix(operatorLogPrefix),
		)
		if err != nil {
			return fmt.Errorf("creating operator logger: %w", err)
		}
		handler, err := operatorLogger.Stream(
			ctx,
			supportlogger.WithSelector(operatorLogSelector),
			supportlogger.WithContainer(operatorLogContainerName),
		)
		if err != nil {
			return fmt.Errorf("starting operator log stream: %w", err)
		}
		b.operatorLogs = handler
	}

	return nil
}

func (b *Base) UninstallOperator(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if err := b.stopOperatorLogs(ctx); err != nil {
		return err
	}
	if b.testCfg == nil || !b.testCfg.Operator.Install {
		return nil
	}
	testCfg := b.testCfg

	helmClient, err := supporthelm.New(b.cfg)
	if err != nil {
		return fmt.Errorf("creating helm client: %w", err)
	}
	if err := helmClient.Uninstall(
		supporthelm.WithUninstallNamespace(testCfg.Operator.Namespace),
		supporthelm.WithUninstallReleaseName(operatorReleaseName),
	); err != nil {
		return err
	}

	return nil
}

func (b *Base) stopOperatorLogs(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if b.operatorLogs == nil {
		return nil
	}
	if err := b.operatorLogs.Stop(ctx); err != nil {
		return err
	}

	b.operatorLogs = nil
	return nil
}
