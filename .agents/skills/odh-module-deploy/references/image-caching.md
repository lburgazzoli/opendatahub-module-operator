# Image Caching

Both the kustomize and Helm deploy paths set `imagePullPolicy: Always`.
When iterating locally with the same tag, Kubernetes still re-pulls. For
extra safety (or if the policy is overridden), use a unique tag per build.

## Ephemeral ttl.sh Flow

Use a fresh short-lived tag per run to avoid stale image cache issues:

```sh
IMG="ttl.sh/$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
make container-build IMG="$IMG"
make container-push IMG="$IMG"
make deploy-helm IMG="$IMG"
```

This ephemeral `ttl.sh` flow is the preferred default for e2e verification.
Using a fresh short-lived tag per run makes debugging deploy/test failures
much more predictable.
