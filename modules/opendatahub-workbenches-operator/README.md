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

## Upgrade tests

Run from this module directory:

```bash
make cleanup-upgrade
make test-upgrade-setup
make test-upgrade-run
make cleanup-upgrade
```

## E2E tests

When using the internal OpenShift registry, do not use `make test-e2e`
directly. That target starts with `cleanup-e2e`, and that cleanup can remove
the namespace and image stream where `push-openshift-image` just published the image.

Use the split flow instead:

```bash
LOCAL_IMG="localhost/opendatahub-workbenches-operator:e2e-$(uuidgen | tr '[:upper:]' '[:lower:]')"

make cleanup-e2e
make container-build IMG="$LOCAL_IMG"

OCP_IMG="$(make push-openshift-image IMG="$LOCAL_IMG")"
echo "Using OpenShift image: $OCP_IMG"

make deploy-helm IMG="$OCP_IMG"
make test-e2e-run
make cleanup-e2e
```

If `make push-openshift-image IMG="$LOCAL_IMG"` fails once, retry it once. If it
fails again, stop there and switch to a `ttl.sh` image instead of continuing
with the internal-registry flow.
