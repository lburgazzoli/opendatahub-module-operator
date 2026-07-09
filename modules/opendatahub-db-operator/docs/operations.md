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

- `<provider>` `StatefulSet`
- `<provider>` `PersistentVolumeClaim`
- `<provider>` headless `Service`
- `<provider>-initdb` `ConfigMap`
- `<provider>` `NetworkPolicy`
- `<provider>-admin` admin Secret

Those resources are created in:

- `spec.embedded.namespace`, if set
- otherwise `operator-namespace`

## Embedded Host Resolution

Claim credentials for an embedded-backed claim always use the Service DNS name,
never a pod IP:

`<provider>.<effective-embedded-namespace>.svc.cluster.local`

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

- claims: `Provisioned` for claim-specific success, `Ready` for aggregate
  top-level status
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

Exactly one of those two modes must be set.

For selector matches:

1. highest `db.infrastructure.opendatahub.io/selection-priority` wins
2. alphabetical provider name breaks ties

If a selector-based claim already has a selected provider in `status.provider`
and that provider still matches, the controller keeps it instead of rebinding to
a newly appearing match.

Provider changes are also watched directly, so creating or updating a provider
can wake pending claims without annotating the claim itself.

## Claim Drift Recovery

Claims reconcile the database-side state they own:

- `SchemaClaim` recreates missing schemas, roles, and credentials Secrets
- `DatabaseClaim` recreates missing roles and credentials Secrets

Claim credentials Secrets are ordinary resources in the claim namespace. They
are managed by reconcile/deploy, but are not owner-referenced to the claim.
Their names default to the claim name, but `spec.secretName` can override the
projected Secret name.

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
