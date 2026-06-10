package config

import (
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func (c *ZapConfig) ZapOpts() []zap.Opts {
	var opts []zap.Opts

	opts = append(opts, zap.UseDevMode(c.DevMode))

	if lvl, err := zapcore.ParseLevel(c.Level); err == nil {
		opts = append(opts, zap.Level(lvl))
	}

	switch c.Encoder {
	case "json":
		opts = append(opts, zap.JSONEncoder())
	case "console":
		opts = append(opts, zap.ConsoleEncoder())
	}

	return opts
}
