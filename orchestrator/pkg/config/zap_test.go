package config

import (
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestZapOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  ZapConfig
		wantLen int
	}{
		{
			name:    "defaults produce dev-mode and level opts",
			config:  ZapConfig{Level: "info"},
			wantLen: 2,
		},
		{
			name:    "dev mode enabled",
			config:  ZapConfig{Level: "debug", DevMode: true},
			wantLen: 2,
		},
		{
			name:    "json encoder adds third opt",
			config:  ZapConfig{Level: "info", Encoder: "json"},
			wantLen: 3,
		},
		{
			name:    "console encoder adds third opt",
			config:  ZapConfig{Level: "warn", Encoder: "console"},
			wantLen: 3,
		},
		{
			name:    "invalid level is skipped",
			config:  ZapConfig{Level: "bogus"},
			wantLen: 1,
		},
		{
			name:    "empty level defaults to info",
			config:  ZapConfig{},
			wantLen: 2,
		},
		{
			name:    "unknown encoder is ignored",
			config:  ZapConfig{Level: "error", Encoder: "unknown"},
			wantLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			opts := tc.config.ZapOpts()
			g.Expect(opts).To(HaveLen(tc.wantLen))

			g.Expect(func() { zap.New(opts...) }).ToNot(Panic())
		})
	}
}
