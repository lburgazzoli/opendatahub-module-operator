package config

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func (c ZapConfig) NewLogger() (logr.Logger, error) {
	level, err := zapcore.ParseLevel(c.Level)
	if err != nil {
		return logr.Logger{}, fmt.Errorf("parsing zap level %q: %w", c.Level, err)
	}

	opts := []ctrlzap.Opts{
		ctrlzap.UseDevMode(c.DevMode),
		ctrlzap.Level(level),
	}

	switch strings.ToLower(c.Encoder) {
	case "":
		// Use controller-runtime defaults: console in dev mode, JSON otherwise.
	case "console":
		opts = append(opts, ctrlzap.ConsoleEncoder())
	case "json":
		opts = append(opts, ctrlzap.JSONEncoder())
	default:
		return logr.Logger{}, fmt.Errorf("unsupported zap encoder %q", c.Encoder)
	}

	return ctrlzap.New(opts...), nil
}
