# opendatahub-db-operator

`opendatahub-db-operator` is the database provisioning module for the ODH module
operator monorepo.

It gives other controllers a Kubernetes-native way to ask for PostgreSQL access:

- `SchemaClaim` provisions a schema and a dedicated user inside an existing
  database.
- `DatabaseClaim` provisions a dedicated user for a pre-existing database.
- `DatabaseProvider` describes where claims should be provisioned:
  - `External`: a PostgreSQL instance managed outside this operator.
  - `Embedded`: a controller-managed single-instance PostgreSQL convenience
    backend.
- `DatabaseService` is the module-enablement CR used by the platform/operator
  layer.

The intent is similar to storage classes and PVCs:

- claims express demand
- providers express supply
- the operator binds the two and writes connection details into a Secret in the
  claim namespace

## Quick Start

The normal flow is:

1. create a `DatabaseProvider`
2. create a `SchemaClaim` or `DatabaseClaim`
3. wait for the claim to become `Provisioned=True`
4. read the generated Secret in the claim namespace

For an embedded provider, the operator also creates the backing PostgreSQL
resources for you. For an external provider, the PostgreSQL instance already
exists and the operator only validates connectivity and provisions access.

## How It Works

The following diagram illustrates how cluster-scoped database providers reconcile with namespaced database or schema claims:

```text
            ┌───────────────────────────────┐
            │   DatabaseProvider (Supply)   │  ◄─── [ Cluster-scoped ]
            │    (Embedded or External)     │
            └───────────────┬───────────────┘
                            ▲
                            │ (References via spec.provider)
                            │
               ┌────────────┴────────────┐
               │                         │
               ▼                         ▼
        ┌─────────────┐           ┌─────────────┐
        │ SchemaClaim │           │DatabaseClaim│  ◄─── [ Namespaced ]
        └──────┬──────┘           └──────┬──────┘
               │                         │
               │ (Reconciled by          │ (Reconciled by
               │  the Operator)          │  the Operator)
               ▼                         ▼
        ┌─────────────┐           ┌─────────────┐
        │   Secret    │           │   Secret    │  ◄─── [ Credentials ]
        │(claim-name) │           │(claim-name) │
        └─────────────┘           └─────────────┘
```

### Claims

`SchemaClaim` and `DatabaseClaim` are namespace-scoped resources.

Both claims:

- select a provider by exact name or by label selector
- requeue when `DatabaseProvider` objects change, so late provider creation and
  selector changes are observed without touching the claim
- keep the current provider for selector-based claims while it still matches
- wait until a single reachable provider is resolved
- provision PostgreSQL roles and grants through the controller
- publish connection details in a Secret named after the claim
- repair missing claim credentials in place; `SchemaClaim` also recreates a
  missing schema
- surface the selected provider in `status.provider` when selection happened by
  label selector
- use `status.conditions[type=Provisioned]` as the main machine-readable signal

Use `SchemaClaim` when the consumer needs its own schema. Use `DatabaseClaim`
when the consumer needs access to a whole existing database.

### Providers

`DatabaseProvider` is cluster-scoped.

#### External

An `External` provider points at an admin-managed PostgreSQL instance. The
operator validates connectivity with an admin Secret and then provisions claims
against that instance.

The referenced Secret must contain:

- `pg.host`
- `pg.port`
- `pg.user`
- `pg.password`
- `pg.database`

#### Embedded

An `Embedded` provider creates and manages:

- a `StatefulSet`
- a `PersistentVolumeClaim`
- a headless `Service`
- an init `ConfigMap`
- a `NetworkPolicy`
- an admin Secret

The embedded provider defaults to creating these resources in the operator
namespace, but `spec.embedded.namespace` can override that. Claim connections
still resolve through the embedded Service DNS name:

`<provider-name>.<target-namespace>.svc.cluster.local`

This is a convenience backend, not a full database service:

- one instance
- no HA
- no backup/restore workflow here
- no arbitrary image override in the CRD

If `spec.embedded.extensions` requests `vector`, the operator selects the
configured pgvector image. Otherwise it uses the configured stock PostgreSQL
image.

### Security & Network Isolation

Because `DatabaseProvider` is cluster-scoped and claims are namespace-scoped, security and tenant isolation are enforced by default:

- **Dynamic Network Isolation (`Embedded` only):** For `Embedded` providers, the operator automatically discovers all namespaces with successfully provisioned claims referencing that provider, and dynamically configures the PostgreSQL `NetworkPolicy` to allow ingress traffic *only* from those specific namespaces.
- **Tenant Credential Isolation:** Generated connection Secrets are created directly in the consumer claim's own namespace. A tenant in namespace `A` cannot access or view the connection credentials generated for a tenant in namespace `B`.

## API Summary

### SchemaClaim

Key fields:

- `spec.provider`
- `spec.schema` optional, immutable
- `spec.access`: `ReadWrite` or `ReadOnly`
- `spec.deletionPolicy`: `Retain` or `Delete`

Status highlights:

- `status.schema`
- `status.connection`
- `status.provider` when the claim resolved a provider by selector
- `status.conditions[type=Provisioned]`

### DatabaseClaim

Key fields:

- `spec.provider`
- `spec.database` required, immutable
- `spec.access`: `ReadWrite` or `ReadOnly`

Status highlights:

- `status.database`
- `status.connection`
- `status.provider` when the claim resolved a provider by selector
- `status.conditions[type=Provisioned]`

### DatabaseProvider

Key fields:

- `spec.type`: `External` or `Embedded`
- `spec.external.connectionSecretRef`
- `spec.embedded.storage`
- `spec.embedded.resources`
- `spec.embedded.extensions`
- `spec.embedded.namespace` optional override for embedded resources
- `spec.embedded.deletionPolicy`

Status highlights:

- `status.conditions[type=Reachable]`

## Examples

Ready-to-apply sample manifests also live in `config/samples/`:

- `config/samples/services_v1alpha1_databaseservice.yaml`
- `config/samples/infrastructure_v1alpha1_databaseprovider_embedded.yaml`
- `config/samples/infrastructure_v1alpha1_databaseprovider_external.yaml`
- `config/samples/infrastructure_v1alpha1_schemaclaim.yaml`
- `config/samples/infrastructure_v1alpha1_databaseclaim.yaml`

### External provider

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: external-postgres-admin
  namespace: opendatahub-db
type: Opaque
stringData:
  pg.host: postgres.example.com
  pg.port: "5432"
  pg.user: postgres
  pg.password: secret
  pg.database: postgres
---
apiVersion: infrastructure.opendatahub.io/v1alpha1
kind: DatabaseProvider
metadata:
  name: shared-external
spec:
  type: External
  external:
    connectionSecretRef:
      name: external-postgres-admin
      namespace: opendatahub-db
```

### Embedded provider

```yaml
apiVersion: infrastructure.opendatahub.io/v1alpha1
kind: DatabaseProvider
metadata:
  name: shared-embedded
  labels:
    db.infrastructure.opendatahub.io/capability-pgvector: "true"
spec:
  type: Embedded
  embedded:
    namespace: opendatahub-db
    deletionPolicy: Retain
    storage:
      size: 10Gi
    extensions:
    - vector
    - pg_trgm
```

This creates a controller-managed PostgreSQL instance in `opendatahub-db`.

### SchemaClaim

```yaml
apiVersion: infrastructure.opendatahub.io/v1alpha1
kind: SchemaClaim
metadata:
  name: notebooks
  namespace: team-a
spec:
  provider:
    name: shared-embedded
  access: ReadWrite
  deletionPolicy: Retain
```

After reconciliation, the Secret `team-a/notebooks` contains the connection
information for the provisioned schema user.

### DatabaseClaim

```yaml
apiVersion: infrastructure.opendatahub.io/v1alpha1
kind: DatabaseClaim
metadata:
  name: ml-metadata
  namespace: team-a
spec:
  provider:
    name: shared-external
  database: mlmd
  access: ReadWrite
```

After reconciliation, the Secret `team-a/ml-metadata` contains the connection
information for the provisioned database user.

## Typical Usage

### Use `SchemaClaim` when

- the application should stay inside a shared database
- each tenant or component should get its own schema
- deleting the claim may or may not delete the schema, depending on
  `deletionPolicy`

### Use `DatabaseClaim` when

- the database already exists
- the application needs its own role on that database
- the operator must not delete the database itself

### Use `External` provider when

- PostgreSQL is already managed elsewhere
- you need functionality outside this module's embedded scope
- you want the operator to provision access, not manage database lifecycle

### Use `Embedded` provider when

- you want a simple in-cluster PostgreSQL backend
- one instance is enough
- you are fine with the module's intentionally limited operational surface

## Connection Secrets

For provisioned claims, the operator writes a Secret in the claim namespace with
the standard PostgreSQL keys used by this module:

- `pg.host`
- `pg.port`
- `pg.user`
- `pg.password`
- `pg.database`

`SchemaClaim` credentials also include:

- `pg.schema`

The Secret name matches the claim name.

More operational details, including resource ownership and embedded namespace
resolution, are in `docs/operations.md`.

## Provider Selection

`spec.provider` supports exactly one of:

- `name`
- `selector`

When a selector matches more than one provider, the operator picks:

1. the provider with the highest
   `db.infrastructure.opendatahub.io/selection-priority` annotation
2. alphabetical name order as a tie-breaker

Once a selector-based claim has picked a provider, the controller keeps that
provider while it still exists and still matches the selector. A newly created
or higher-priority match does not force rebinding of an already-bound claim.

```mermaid
flowchart TD
    Start([Reconcile Claim]) --> IsBound{Is Claim Already Bound?}
    
    IsBound -- Yes --> StillMatches{Does Bound Provider Exist\n& Selector Still Match?}
    StillMatches -- Yes --> KeepBinding[Keep Current Provider] --> End([End Reconciliation])
    StillMatches -- No --> ClearBinding[Clear Existing Binding] --> Resolve
    
    IsBound -- No --> Resolve

    Resolve{Resolve spec.provider}
    Resolve -->|Exact Name| DirectName[Bind Directly by Name] --> End
    Resolve -->|Label Selector| FindMatches[Query All Providers Matching Selector]
    
    FindMatches --> AnyMatches{Any Providers Matched?}
    AnyMatches -- No --> Requeue[Requeue: Wait for Match] --> End
    
    AnyMatches -- Yes (1 Match) --> BindSingle[Bind to Single Match] --> End
    
    AnyMatches -- Yes (Multiple Matches) --> SortPriority[Compare 'selection-priority' Annotation]
    SortPriority --> TieCheck{Is There a Priority Tie?}
    TieCheck -- No --> BindHighest[Bind to Highest Priority Provider] --> End
    TieCheck -- Yes --> Alphabetical[Tie-breaker: Alphabetical Order by Name] --> BindFirst[Bind to First Alphabetical] --> End
```

## Drift Recovery

Claims repair the resources they own when drift is detected:

- `SchemaClaim` recreates a missing schema, role, or credentials Secret
- `DatabaseClaim` recreates a missing role or credentials Secret

Repairs amend the existing claim Secret in place when possible. Credentials only
rotate when the controller has to reprovision the missing database-side state.

## Local Development

Run commands from `modules/opendatahub-db-operator/`.

### Prerequisites

To build, run, and test the operator locally, you will need:

- **Go:** `1.26.4` or later (as specified in `go.mod`)
- **Container Tool:** `podman` (default) or `docker` (configured via `CONTAINER_TOOL` environment variable)
- **Kubernetes Cluster:** A running local cluster (such as Kind, Minikube, or OpenShift Local) with `kubectl` configured and targeted by your active context

### Useful Targets

Useful targets:

- `make manifests generate`
- `make fmt`
- `make lint`
- `make test`
- `make test-integration`
- `make install`
- `make run`
- `make helm`
- `make deploy-helm`

### Configuration

Configuration is loaded from:

1. built-in defaults
2. files under `ODH_MODULE_OPERATOR_CONFIGURATION_PATH`
3. environment variables prefixed with `ODH_MODULE_OPERATOR_`

Important keys:

- `operator-namespace`
- `embedded.postgres-image`
- `embedded.pgvector-image`
- `grace-period`
- `schemaclaim.retry-interval`
- `databaseclaim.retry-interval`
- `databaseprovider.retry-interval`
- `databaseservice.retry-interval`

Example overrides:

```bash
export ODH_MODULE_OPERATOR_OPERATOR_NAMESPACE=odh-db-operator-system
export ODH_MODULE_OPERATOR_DATABASEPROVIDER_RETRY_INTERVAL=2m
export ODH_MODULE_OPERATOR_EMBEDDED_POSTGRES_IMAGE=postgres:16
export ODH_MODULE_OPERATOR_EMBEDDED_PGVECTOR_IMAGE=pgvector/pgvector:pg16
```

## Without Cloning

You can also use the module directly from GitHub.

Install CRDs from the repo:

```bash
kubectl apply -k "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/config/crd?ref=db-service"
```

Apply the sample resources from the repo:

```bash
kubectl apply -k "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/config/samples?ref=db-service"
```

Run the operator directly with Go:

```bash
go run github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator@db-service operator
```

If you use a different branch or tag, replace `db-service` with that ref.

## Notes For Maintainers

- Generated files must be refreshed with `make manifests generate` after API
  changes.
- Do not hand-edit generated CRD YAML or deepcopy files.
- `config/crd/kustomization.yaml` must include the generated CRD bases for
  `make install` to work.
