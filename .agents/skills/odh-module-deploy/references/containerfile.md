# Containerfile and Manifest Permissions

## Build Split

`make container-prep` runs on the host (`manifests`, `generate`,
`get-manifests` for fetch modules). The Containerfile only runs
`make build-bin` to compile the manager -- generation and manifest fetch stay
off the critical path inside the image layer cache.

## OpenShift Arbitrary UIDs

OpenShift assigns arbitrary UIDs to containers. Manifests baked into the image
must be world-readable so the init container (which copies them to a writable
emptyDir) can access them regardless of the assigned UID.

In the Containerfile:

```dockerfile
# In the builder stage -- set permissions before copying to runtime
RUN chmod -R a+rX config/manifests/

# In the runtime stage -- copy from builder (preserves permissions)
COPY --from=builder /workspace/config/manifests/ /manifests/
```

## Init Container with emptyDir

The manager Deployment uses an init container to copy manifests to a writable
volume, because `fwparams.Apply` (called by `module.Init`) writes to `params.env`
in-place at operator startup:

```yaml
initContainers:
- name: copy-manifests
  image: controller:latest
  command: ["cp", "-r", "/manifests/.", "/opt/manifests/"]
  volumeMounts:
  - name: manifests
    mountPath: /opt/manifests
containers:
- name: manager
  volumeMounts:
  - name: manifests
    mountPath: /opt/manifests
  env:
  - name: ODH_MODULE_OPERATOR_MANIFESTS_PATH
    value: /opt/manifests
volumes:
- name: manifests
  emptyDir: {}
```
