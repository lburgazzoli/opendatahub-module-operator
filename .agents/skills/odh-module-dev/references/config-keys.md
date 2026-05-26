# Adding a Config Key

Follow these 5 steps to add a new configuration key to a module operator:

1. **Add constant:** `KeyMyField = "my-field"` in `pkg/config/config.go`
2. **Add field:** `MyField string \`mapstructure:"my-field"\`` in `Config` struct
3. **Add default:** `v.SetDefault(KeyMyField, "default-value")` in `setDefaults()`
4. **Add to ConfigMap:** add the key to `config/manager/configmap.yaml` data
5. **Env var is automatic:** becomes `ODH_MODULE_OPERATOR_MY_FIELD` -- never
   hand-craft a different prefix

The key must be known to viper (via `SetDefault`, config file load, or explicit
`BindEnv`) before `bindEnv()` runs in `Load()`. Otherwise the env var is
silently ignored during `Unmarshal()`.

After adding a key used in integration tests, update
`config/manager/configmap.yaml` -- tests derive expected values from it via
`support.MustReadConfigMapData()`.
