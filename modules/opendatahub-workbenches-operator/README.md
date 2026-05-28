# opendatahub-workbenches-operator

## Test namespaces

Default namespaces used by this module:

- `OPERATOR_NAMESPACE=opendatahub-workbenches-system`
- `INTEGRATION_TEST_NAMESPACE=opendatahub-workbenches-integration-tests`

You can override them with environment variables, but the manual commands below
assume the defaults.

## Integration tests

Run from this module directory:

```bash
make cleanup-integration
make test-integration-setup
make test-integration-run
make cleanup-integration
```

Single-command variant that preserves the test exit code and always cleans up:

```bash
make cleanup-integration && \
make test-integration-setup && \
make test-integration-run; \
STATUS=$?; \
make cleanup-integration; \
exit $STATUS
```

## Upgrade tests

Run from this module directory:

```bash
make cleanup-upgrade
make test-upgrade-setup
make test-upgrade-run
make cleanup-upgrade
```

Single-command variant that preserves the test exit code and always cleans up:

```bash
make cleanup-upgrade && \
make test-upgrade-setup && \
make test-upgrade-run; \
STATUS=$?; \
make cleanup-upgrade; \
exit $STATUS
```

## E2E tests

When using the internal OpenShift registry, do not use `make test-e2e`
directly. That target starts with `cleanup-e2e`, and that cleanup can remove
the namespace and image stream where `push-crc-image` just published the image.

Use the split flow instead:

```bash
LOCAL_IMG="localhost/opendatahub-workbenches-operator:e2e-$(uuidgen | tr '[:upper:]' '[:lower:]')"

make cleanup-e2e
make container-build IMG="$LOCAL_IMG"

CRC_IMG="$(make push-crc-image IMG="$LOCAL_IMG")"
echo "Using CRC image: $CRC_IMG"

make deploy-helm IMG="$CRC_IMG"
make test-e2e-run
make cleanup-e2e
```

Single-command variant that preserves the test exit code and always cleans up:

```bash
LOCAL_IMG="localhost/opendatahub-workbenches-operator:e2e-$(uuidgen | tr '[:upper:]' '[:lower:]')" && \
make cleanup-e2e && \
make container-build IMG="$LOCAL_IMG" && \
CRC_IMG="$(make push-crc-image IMG="$LOCAL_IMG")" && \
echo "Using CRC image: $CRC_IMG" && \
make deploy-helm IMG="$CRC_IMG" && \
make test-e2e-run; \
STATUS=$?; \
make cleanup-e2e; \
exit $STATUS
```

If `make push-crc-image IMG="$LOCAL_IMG"` fails once, retry it once. If it
fails again, stop there and switch to a `ttl.sh` image instead of continuing
with the internal-registry flow.
