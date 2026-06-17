# Webhook Logic

Source: `/home/luca/work/dev/openshift-ai/opendatahub-operator/internal/webhook/`

## Per-Component Webhook Mapping

| Component | Webhook Type | What it does | Copy to module? |
|-----------|-------------|-------------|-----------------|
| dashboard | Validating | AcceleratorProfile deprecation warnings | Yes (if AP migration needed) |
| kserve | Mutating | InferenceService connection injection (S3/OCI/URI) | Yes |
| kserve | Mutating | LLM InferenceService connection handling | Yes |
| workbenches | Mutating | Notebook pod mutation (hardware profile, connections) | Yes |
| kserve + workbenches | Mutating | Hardware profile injection into Pods/Workloads | Yes (shared — each module copies relevant parts) |
| kueue | Validating | Label validation for workloads | No (currently DISABLED) |
| monitoring | Mutating | ServiceMonitor label injection | No (global, not per-component) |

## Simple Components — No Webhooks

trainingoperator, ray, sparkoperator, feastoperator, ogx, mlflowoperator,
trustyai, trainer, datasciencepipelines, modelregistry, modelcontroller,
modelsasservice — none of these have component-specific webhooks.

## Module Webhook Pattern

Each module registers webhooks in `NewReconciler()` (see
`modules/opendatahub-ray-operator/internal/controller/ray/ray_webhook.go` for
the pattern):

```go
if cfg.WebhooksEnabled {
    if err := m.RegisterWebhooks(mgr); err != nil {
        return err
    }
}
```

Webhooks can be disabled via config for local development without TLS certs.
