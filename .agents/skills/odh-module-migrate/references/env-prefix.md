# Env Prefix Rule

**Rule:** `ODH_MODULE_OPERATOR_` for every module operator — identical to the
example module. Never use `ODH_OPERATOR_`. Never embed the component name
(e.g. `ODH_RAY_OPERATOR_*`). `pkg/config.EnvPrefix` and `ConfigPathEnvVar`
must match deployment env vars and `make run`.

Copy from the ray module template
`modules/opendatahub-ray-operator/pkg/config/config.go` and
`modules/opendatahub-ray-operator/config/manager/manager.yaml`.

These files must stay in sync:

| File | Required env vars |
|------|-------------------|
| `pkg/config/config.go` | `EnvPrefix`, `ConfigPathEnvVar` |
| `config/manager/manager.yaml` | `_CONFIGURATION_PATH`, `_MANIFESTS_PATH`, `_APPLICATIONS_NAMESPACE` |
| `config/default/manager_metrics_patch.yaml` | `_METRICS_BIND_ADDRESS` |
| `config/chart/templates/apps_v1_deployment.yaml` | same as manager |
| `Makefile` `run` target | `ODH_MODULE_OPERATOR_MANIFESTS_PATH=...` |

`RHAI_APPLICATIONS_NAMESPACE` is separate — set in deployments for
opendatahub-operator framework compatibility, not part of module config.

Verification grep (run from module root):

```bash
rg 'ODH_OPERATOR[^_M]|ODH_OPERATOR_|ODH_[A-Z]+_OPERATOR_' . && exit 1 || true
```
