package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
)

type upperTextValue string

func (t *upperTextValue) UnmarshalText(text []byte) error {
	*t = upperTextValue(strings.ToUpper(string(text)))
	return nil
}

func TestBindEnv_ExposesEnvOverridesToDecode(t *testing.T) {
	g := NewWithT(t)

	type nested struct {
		Value string `mapstructure:"value"`
	}
	type cfg struct {
		Foo nested `mapstructure:"foo"`
	}

	t.Setenv("ODH_MODULE_OPERATOR_TEST_HELPER_FOO_VALUE", "from-env")

	v := viper.New()
	v.SetDefault("foo.value", "default")

	err := config.BindEnv(
		v,
		"ODH_MODULE_OPERATOR_TEST_HELPER",
		strings.NewReplacer(".", "_", "-", "_"),
		"foo.value",
	)
	g.Expect(err).NotTo(HaveOccurred())

	got := &cfg{}
	g.Expect(config.Decode(v, got)).To(Succeed())
	g.Expect(got.Foo.Value).To(Equal("from-env"))
}

func TestDecode_UsesDefaultDurationHook(t *testing.T) {
	g := NewWithT(t)

	type cfg struct {
		Timeout time.Duration `mapstructure:"timeout"`
	}

	v := viper.New()
	v.Set("timeout", "15s")

	got := &cfg{}
	err := config.Decode(v, got)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Timeout).To(Equal(15 * time.Second))
}

func TestDecode_UsesProvidedHooks(t *testing.T) {
	g := NewWithT(t)

	type cfg struct {
		Value upperTextValue `mapstructure:"value"`
	}

	v := viper.New()
	v.Set("value", "custom")

	got := &cfg{}
	err := config.Decode(
		v,
		got,
		mapstructure.TextUnmarshallerHookFunc(),
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Value).To(Equal(upperTextValue("CUSTOM")))
}
