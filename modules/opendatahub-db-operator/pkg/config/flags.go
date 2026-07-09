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

package config

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	durationType        = reflect.TypeFor[time.Duration]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// RegisterFlags derives Cobra flags from the Config struct's mapstructure tags,
// registers them on cmd, and binds each flag to the same Viper key used by file
// and env-based config loading.
func RegisterFlags(cmd *cobra.Command, v *viper.Viper) error {
	if cmd == nil {
		return fmt.Errorf("command is required")
	}
	if v == nil {
		return fmt.Errorf("viper is required")
	}

	setDefaults(v)

	return registerStructFlags(cmd.Flags(), v, reflect.TypeFor[Config](), nil)
}

func registerStructFlags(
	flags *pflag.FlagSet,
	v *viper.Viper,
	t reflect.Type,
	prefix []string,
) error {
	for field := range t.Fields() {
		if !field.IsExported() {
			continue
		}

		tag := mapstructureName(field)
		if tag == "" {
			continue
		}

		keyParts := append(append([]string(nil), prefix...), tag)
		fieldType := field.Type

		if isNestedConfigStruct(fieldType) {
			if err := registerStructFlags(flags, v, fieldType, keyParts); err != nil {
				return err
			}
			continue
		}

		key := strings.Join(keyParts, ".")
		flagName := flagNameForKey(keyParts)
		help := fmt.Sprintf("Override config key %q", key)

		switch {
		case fieldType.Kind() == reflect.String:
			flags.String(flagName, v.GetString(key), help)
		case fieldType.Kind() == reflect.Bool:
			flags.Bool(flagName, v.GetBool(key), help)
		case fieldType == durationType:
			flags.Duration(flagName, v.GetDuration(key), help)
		case isTextUnmarshaler(fieldType):
			flags.String(flagName, fmt.Sprint(v.Get(key)), help)
		default:
			continue
		}

		if err := v.BindPFlag(key, flags.Lookup(flagName)); err != nil {
			return fmt.Errorf("binding flag %q to key %q: %w", flagName, key, err)
		}
	}

	return nil
}

func mapstructureName(field reflect.StructField) string {
	tag := field.Tag.Get("mapstructure")
	if tag == "" {
		return ""
	}

	name, _, _ := strings.Cut(tag, ",")
	if name == "" || name == "-" {
		return ""
	}

	return name
}

func isNestedConfigStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t != durationType && !isTextUnmarshaler(t)
}

func isTextUnmarshaler(t reflect.Type) bool {
	return t.Implements(textUnmarshalerType) || reflect.PointerTo(t).Implements(textUnmarshalerType)
}

func flagNameForKey(keyParts []string) string {
	parts := make([]string, 0, len(keyParts))
	for _, part := range keyParts {
		parts = append(parts, kebabCase(part))
	}
	return strings.Join(parts, "-")
}

func kebabCase(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	for i, r := range s {
		if r == '.' {
			b.WriteByte('-')
			continue
		}

		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rune(s[i-1])
				if prev != '-' && prev != '_' && !unicode.IsUpper(prev) {
					b.WriteByte('-')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}

		if r == '_' {
			b.WriteByte('-')
			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}
