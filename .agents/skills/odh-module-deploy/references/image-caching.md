# Image Caching

Both the kustomize and Helm deploy paths set `imagePullPolicy: Always`.
When iterating locally with the same tag, Kubernetes still re-pulls. For
extra safety (or if the policy is overridden), use a unique tag per build.

## Ephemeral ttl.sh Flow

Use a fresh short-lived tag per run to avoid stale image cache issues:

```sh
cd modules/opendatahub-mymodule-operator
IMG=ttl.sh/opendatahub-mymodule-operator-$(uuidgen):1h \
  make container-build container-push deploy-helm
```

This ephemeral `ttl.sh` flow is the preferred default for e2e verification.
Using a fresh short-lived tag per run makes debugging deploy/test failures
much more predictable.
