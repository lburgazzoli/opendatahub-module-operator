# Database Operator Operations

This note complements `README.md` with the concrete details that are most useful
when operating or extending this module.

## Resource Ownership

### External provider

`DatabaseProvider.spec.type=External` owns no PostgreSQL runtime resources.

It only:

- reads the referenced admin Secret
- checks reachability
- exposes `status.conditions[type=Reachable]`
- serves as the provisioning target for claims

It does not create:

- `StatefulSet`
- `PersistentVolumeClaim`
- `Service`
- `ConfigMap`
- `NetworkPolicy`

### Embedded provider

`DatabaseProvider.spec.type=Embedded` owns the PostgreSQL runtime resources.

It creates:

- `<provider>-postgres` `StatefulSet`
- `<provider>-postgres` `PersistentVolumeClaim`
- `<provider>-postgres` headless `Service`
- `<provider>-postgres-initdb` `ConfigMap`
- `<provider>-postgres` `NetworkPolicy`
- `<provider>-admin` admin Secret

Those resources are created in:

- `spec.embedded.namespace`, if set
- otherwise `operator-namespace`

## Embedded Host Resolution

Claim credentials for an embedded-backed claim always use the Service DNS name,
never a pod IP:

`<provider>-postgres.<effective-embedded-namespace>.svc`

where effective embedded namespace means:

- `spec.embedded.namespace`, if set
- otherwise `operator-namespace`

## Secret Formats

### External admin secret

Expected keys:

- `pg.host`
- `pg.port`
- `pg.user`
- `pg.password`
- `pg.database`

### Provisioned claim secret

Common keys:

- `pg.host`
- `pg.port`
- `pg.user`
- `pg.password`
- `pg.database`

Additional key for `SchemaClaim`:

- `pg.schema`

### Embedded admin secret

The embedded provider uses keys that match the upstream PostgreSQL container
environment variables:

- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_DB`

## Conditions

Primary conditions to watch:

- claims: `Provisioned`
- providers: `Reachable`
- module CR: `Ready`

Common embedded `Reachable` reasons include:

- `Provisioning`
- `InstanceRunning`
- `ImageUnmapped`
- `ExtensionChangeRequiresRecreate`
- `AdminSecretUnavailable`
- `Idle`

## Provider Selection Notes

Claims can select providers by:

- exact name
- label selector

For selector matches:

1. highest `db.infrastructure.opendatahub.io/selection-priority` wins
2. alphabetical provider name breaks ties

A provider can also be marked as the default with:

`db.infrastructure.opendatahub.io/is-default-provider: "true"`

## Local Verification

Common commands from `modules/opendatahub-db-operator/`:

```bash
make manifests generate
make lint
make test
make test-integration
make install
make run
```

If you change API types or kubebuilder markers, regenerate before reviewing the
diff:

```bash
make manifests generate
```
