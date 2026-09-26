# Configuration

ThinkPixelWS process configuration is loaded by `internal/config`. Values are
applied in this order, with later sources taking precedence:

1. safe built-in defaults;
2. an optional JSON configuration file;
3. explicit `THINKPIXELWS_*` environment variables.

`LoadFromEnvironment` reads the optional file path from
`THINKPIXELWS_CONFIG_FILE`. Unknown JSON fields, malformed values, unsafe
limits, empty bind hosts, and invalid secret references are rejected at
startup.

## Defaults and environment variables

| JSON field | Environment variable | Default |
|---|---|---|
| `database_url_file` | `THINKPIXELWS_DATABASE_URL_FILE` | empty (Workspace API disabled) |
| `cursor_key_file` | `THINKPIXELWS_CURSOR_KEY_FILE` | empty (random process-local key) |
| `auth.mode` | `THINKPIXELWS_AUTH_MODE` | `disabled` |
| `auth.tenant_id` | `THINKPIXELWS_AUTH_TENANT_ID` | empty |
| `auth.principal` | `THINKPIXELWS_AUTH_PRINCIPAL` | empty |
| `auth.token_file` | `THINKPIXELWS_AUTH_TOKEN_FILE` | empty |
| `http.listen_address` | `THINKPIXELWS_HTTP_LISTEN_ADDRESS` | `127.0.0.1:8080` |
| `http.read_header_timeout` | `THINKPIXELWS_HTTP_READ_HEADER_TIMEOUT` | `5s` |
| `http.request_timeout` | `THINKPIXELWS_HTTP_REQUEST_TIMEOUT` | `30s` |
| `http.shutdown_timeout` | `THINKPIXELWS_HTTP_SHUTDOWN_TIMEOUT` | `15s` |
| `http.max_header_bytes` | `THINKPIXELWS_HTTP_MAX_HEADER_BYTES` | `65536` |
| `log.level` | `THINKPIXELWS_LOG_LEVEL` | `info` |
| `metrics.listen_address` | `THINKPIXELWS_METRICS_LISTEN_ADDRESS` | `127.0.0.1:9090` |
| `secret_references` | `THINKPIXELWS_SECRET_REFERENCES` | `{}` |

Durations use Go duration syntax such as `500ms`, `30s`, or `2m`. Log levels
are `debug`, `info`, `warn`, and `error`. Network listeners require an explicit
host and numeric port. Loopback defaults prevent accidental network exposure;
a deployment must explicitly select a non-loopback listener.

Example file:

```json
{
  "http": {
    "listen_address": "127.0.0.1:8080",
    "request_timeout": "30s"
  },
  "log": {
    "level": "info"
  },
  "secret_references": {
    "database-password": {
      "provider": "kubernetes",
      "reference": "thinkpixelws/database#password"
    }
  }
}
```

## Secret boundary

A secret reference contains a provider name and an opaque provider-specific
locator. It MUST NOT contain the credential itself. Configuration loading does
not resolve references, read credential-valued environment variables, or place
resolved values in the `Config` object. A future secret-provider adapter owns
resolution at the trusted use boundary.

`THINKPIXELWS_SECRET_REFERENCES`, when used, is a JSON object with the same
shape as the file field. Provider implementations remain replaceable; the
configuration package does not assign authority or embed provider SDKs.

## Development authentication

Opt in explicitly for a trusted local machine. Generate a token outside the
repository and Workspace content; no shared/default credential is provided:

```sh
umask 077
mkdir -p "$HOME/.config/thinkpixelws"
python3 -c 'import secrets; print(secrets.token_urlsafe(32))' > "$HOME/.config/thinkpixelws/dev-token"
export THINKPIXELWS_AUTH_MODE=development
export THINKPIXELWS_AUTH_TENANT_ID=0198cb17-3520-7000-8000-000000000001
export THINKPIXELWS_AUTH_PRINCIPAL=local-developer
export THINKPIXELWS_AUTH_TOKEN_FILE="$HOME/.config/thinkpixelws/dev-token"
go run ./cmd/thinkpixelws
```

The tenant must be an explicit canonical UUIDv7. This does not provision a tenant.
The principal follows the same validation as OIDC claim mapping. Both HTTP and
metrics listeners must use literal loopback IPs (`127.0.0.1` or `::1`); wildcard
addresses and hostnames are rejected. Never expose this mode through a proxy or
tunnel. It does not distinguish local users who possess the token.

The token file must contain one random ASCII token of 32–256 bytes, with optional
surrounding whitespace. It is read once at startup, with a 4096-byte file limit;
invalid or missing configuration fails startup. Restart after token rotation.
Requests use `Authorization: Bearer TOKEN`; the existing CLI can use the same
file through `THINKPIXELWS_TOKEN_FILE`. Never put the token itself in configuration,
logs, or Workspace files.

Development policy allows only Workspace create/list/view for the configured
identity. Handlers receive `httpserver.IdentityFromContext` and must call
`httpserver.AuthorizeWorkspace` with the operation and tenant-resolved target.
All other actions remain denied, including materialization and profile access.
A development-mode warning is logged at startup. Health/readiness and metrics
remain unauthenticated. Workspace APIs additionally require PostgreSQL
configuration below.

`disabled` disables development auth; it does not enable anonymous API access.
Configured API handlers return 503 until authentication is configured. There is
no production OIDC HTTP mode yet. Unknown modes and identity/token settings with
`disabled` are rejected rather than silently ignored.


## Workspace API and PostgreSQL

Workspace create/list/get and component/generation read APIs are available when `THINKPIXELWS_DATABASE_URL_FILE` points
to a file containing the PostgreSQL connection URL. Keep this file outside the
repository and Workspace content, readable only by the service operator. The
service reads it at startup, verifies connectivity, and uses PostgreSQL for
readiness checks. Missing or invalid files and failed connections abort startup
without printing the URL. Leave the setting empty to run health/metrics only.
Use `sslmode=verify-full` with a trusted CA for a remote database.

For local development, first [start PostgreSQL and apply migrations](database-development.md),
then provision the development tenant explicitly (using the local Compose
migration account):

```sh
docker compose exec -T postgres psql -U thinkpixelws_migrator -d thinkpixelws <<'SQL'
INSERT INTO thinkpixelws.tenants (tenant_id, lifecycle_state)
VALUES ('0198cb17-3520-7000-8000-000000000001', 'ACTIVE')
ON CONFLICT DO NOTHING;
SQL
```

Set up development authentication above. Write the connection URL to a private
file using your local database credentials, then configure its path and start:

```sh
export THINKPIXELWS_DATABASE_URL_FILE="$HOME/.config/thinkpixelws/database-url"
go run ./cmd/thinkpixelws
```

Service startup does not migrate the database or create tenants. Use a separately
provisioned database role for deployments; the local migration account is a
development convenience.

Create a Workspace without putting the bearer token in process arguments:

```sh
python3 - <<'PYTHON'
import json, os, pathlib, urllib.request, uuid
body = {"name": "demo", "owner": {"kind": "user", "id": "local-developer"}}
token = pathlib.Path(os.environ["THINKPIXELWS_AUTH_TOKEN_FILE"]).read_text().strip()
request = urllib.request.Request(
    "http://127.0.0.1:8080/v1/workspaces",
    data=json.dumps(body).encode(),
    headers={"Content-Type": "application/json", "Authorization": "Bearer " + token,
             "Idempotency-Key": str(uuid.uuid4())})
with urllib.request.urlopen(request) as response:
    print(response.status, response.headers["Location"])
    print(response.read().decode())
PYTHON
```

The response is `201 Created` with a UUIDv7 ID, `Location`, state `CREATING`,
state version 1, and default classification `internal`. Creation establishes
metadata only; it does not provision storage, advance to READY, create a
Generation, or grant execution authority. Owner metadata is descriptive and may
differ from the authenticated principal.

Keep the same 16–128-character `Idempotency-Key` when retrying the same operation.
Keys are scoped to authenticated tenant, principal, and operation; only their
SHA-256 hashes are stored. JSON property order, omitted default classification,
and residency label order do not change request identity. A matching retry
returns the original creation response, including after a process restart;
a changed request with the same key returns 409. Workspace, audit, outbox, and
idempotency records commit atomically. Records have a 24-hour retention floor;
automatic expiry cleanup is not implemented, so retained keys keep replaying.

Read current metadata with `GET /v1/workspaces/{workspace_id}` and enumerate it
with `GET /v1/workspaces?limit=100`, using the same bearer token. Get returns 404
for an absent Workspace or one in another tenant. It includes `headGeneration`
only when a committed head exists.

List defaults to 100 records, accepts 1–500, and orders by ascending Workspace
UUID. Pass a returned `nextCursor` as the `cursor` query parameter until it is
absent. Cursors are authenticated, bound to the tenant/principal/list operation,
and expire after 15 minutes (410); malformed, modified, or wrong-scope cursors
return 400. Pagination is a live keyset traversal, not a snapshot: deletion does
not shift offsets, and new rows after the boundary may appear on later pages.
Every request checks list permission and view permission for each candidate.
Denied items are omitted, so a page can be short or empty while still having a
`nextCursor`. Development policy allows all metadata within the configured tenant.

Read component and generation metadata through:

- `GET /v1/workspaces/{workspace_id}/components`
- `GET /v1/workspaces/{workspace_id}/components/{component_id}`
- `GET /v1/workspaces/{workspace_id}/generations`
- `GET /v1/workspaces/{workspace_id}/generations/{generation}`

Each request resolves the parent in the authenticated tenant and requires
Workspace view permission before reading child metadata. Missing parents,
cross-tenant parents, and children outside the specified Workspace return 404;
a denied view returns 403.

These lists return JSON arrays (including `[]` when empty). Both accept
`limit` (default 100, maximum 500) and `cursor`. Pass the optional `Next-Cursor`
response header as the next request's `cursor`; its absence ends traversal.
Components sort by ascending UUID and generations by ascending number. These
cursors additionally bind the Workspace and resource type, with the same
15-minute lifetime and signing-key configuration as Workspace pagination.
Every page checks current authorization. Traversal is live, not a snapshot.

Components expose stable identity, canonical path, and the current source
binding when present. Classification and taints come from the current committed
head; absent classification metadata is omitted rather than inferred. These
endpoints do not return component contents or historical component snapshots.
Generations expose persisted immutable metadata, including digest, durability,
and parent number when present. A newly created Workspace has empty lists;
component creation and generation commit APIs remain pending.

By default, startup generates a random cursor signing key; a restart invalidates
outstanding cursors. To preserve cursors across restarts or replicas, set
`THINKPIXELWS_CURSOR_KEY_FILE` to the same private external file containing
exactly 32 cryptographically random raw bytes. Keep it outside this repository
and Workspace content. For example, provision it once with `openssl rand -out
/path/to/private/cursor-key 32` under `umask 077`. A configured unreadable or
incorrectly sized key aborts startup; rotating the key invalidates old cursors.

The focused PostgreSQL integration test creates and drops an isolated database.
Point it at a disposable local instance using a URL for an account with CREATEDB:

```sh
THINKPIXELWS_TEST_DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/postgres?sslmode=disable' \
  go test -race ./internal/adapters/httpserver -run 'Test(CreateWorkspace|ReadWorkspaces|ReadWorkspaceMetadata)Postgres' -v
```

## Kubernetes working-storage client

`internal/adapters/workingstorage/kubernetes.New` accepts trusted operator
configuration: `Kubeconfig` (an explicit local secret-file path), optional `Context`,
required `Namespace`, and `Timeout` (default 30 seconds, maximum 5 minutes).
With no kubeconfig path, it uses the WS Pod's in-cluster service account. It does
not implicitly select `$KUBECONFIG` or a developer's default kubeconfig, and a
failed explicit configuration does not fall back to another cluster.

Kubeconfigs may invoke credential plugins; accept them only from the operator,
never from Workspace contents or API callers. Credentials stay in WS client
transports. The namespace scopes provider resource requests; Kubernetes RBAC must
limit the service account independently, and AG authorization is still required.

The client constructor supplies core-v1 and discovery clients without contacting
the cluster or creating resources. `NewProvider(ctx, client, ProviderConfig)`
constructs the Kubernetes `WorkingStorageProvider` for one operator-configured
`TargetID` and selects `Profile` from the operator-provided `Profiles` map. Each
`StorageProfile` has an explicit `StorageClass`, positive `Capacity` (for example
`1Gi`), and optional `AccessMode` (default `ReadWriteOnce`; alternatively
`ReadWriteOncePod` with a supporting CSI driver). Unknown profile names, invalid
selected profiles, and shared/read-only access modes are rejected before API I/O.
Only the selected profile is validated and copied at construction; changing the
configuration map afterward cannot alter an existing provider.

For example, construct the adapter with:

```go
kubernetes.ProviderConfig{
    TargetID: "demo",
    Profile:  "durable",
    Profiles: map[string]kubernetes.StorageProfile{
        "durable": {StorageClass: "csi-ssd", Capacity: "8Gi"},
    },
}
```

Use an installed StorageClass name; the example does not provision a CSI driver.
A nonempty Materialization target storage class must match the selected profile.
The profile is operator-selected, not supplied by Workspace content or API callers.
It checks the namespaced core PVC API at construction and before each operation.
This is API availability discovery, not a check of RBAC permissions or CSI
snapshot/clone support.

The provider allocates an empty filesystem PVC with the selected access mode, reports its
storage phase, and requests deletion of that PVC. It uses deterministic names,
tenant/Workspace/Materialization ownership metadata, and handles tied to PVC UIDs.
Allocation records the profile name in a PVC annotation. Retries reject profile,
ownership, or specification conflicts; changing a profile does not resize an
existing PVC. Status and release remain available after capacity/access-mode or
profile-name changes when the target and storage class still match. Release checks
ownership and uses UID/resource-version preconditions; absent storage is already
released, while terminating storage is still reported as releasing. PVCs have no
sandbox owner reference, so sandbox deletion does not trigger their garbage
collection. Actual volume deletion follows the configured StorageClass reclaim
policy and Kubernetes finalizers.

These are storage primitives: a bound PVC does not mean Workspace content has
been restored or a Materialization is READY. `ReadWriteOnce` does not enforce the
WS writer lease or read-only execution. The caller must authorize operations,
load tenant-scoped records, persist the returned opaque handle, and detach
execution before release. No credentials are included in handles. The provider
has no database or portable-store access.

`internal/adapters/workingstorage/layout.Prepare` creates component directories
in a pre-opened filesystem volume root that will be mounted at `/workspace`.
Each component uses its validated name as the volume-relative directory and
retains its canonical `/workspace/<name>` execution path. Results are ordered by
name regardless of input order. All component kinds use the same layout.
Retries preserve existing directories and contents; duplicate names/IDs,
cross-tenant/Workspace records, noncanonical paths, files, and symlinks at component
roots are rejected. Unselected directories are left untouched. New directories
use mode `0755` subject to the process umask; existing permissions are preserved.
The caller supplies trusted generation membership and holds the mounted storage
exclusively with execution detached. The helper creates directories only: it
does not restore content, enforce leases, or make a Materialization READY.
It supports both local-path and CSI filesystem volumes. Tests exercise a local
filesystem, including reopening storage and retrying with existing content.

`internal/app/materialization.Preparer` coordinates an already-authorized request
through `REQUESTED → PREPARING → READY`. It persists the opaque PVC handle before
content work, resumes from a persisted binding, and uses the existing repository
state/version compare-and-swap for lifecycle changes. Pending mounting returns
`PREPARING`; errors retain progress and storage for retry or authorized cleanup.
A ready replay returns the existing record without restoring over working edits.
Lost, replaced, or releasing storage cannot become ready. Preparation does not
acquire a writer lease, activate execution, or publish an AR binding.

The coordinator requires a `MaterializationContentPreparer`. The mounted adapter
in `internal/adapters/workingstorage/mounted` composes two trusted dependencies:
`WithRoot`, which exclusively mounts the exact volume with execution detached,
and `Restore`, which restores/verifies the selected generation and supplies its
complete component membership. It rechecks the persisted request after obtaining
exclusive access, restores content, invokes the layout helper, and persists
readiness before releasing exclusivity. Restoring must be idempotent after partial
failure. A missing restore implementation fails; an empty PVC is never implicitly
treated as a restored generation. `WithRoot` may start a scheduling consumer while
storage is pending, which avoids blocking `WaitForFirstConsumer` provisioning.
Readiness still requires the same PVC UID to be bound after preparation.

This is an internal orchestration path, not a deployed prepare endpoint. Process/API
wiring, a Kubernetes mount worker, the generation restore backend, AR attachment,
and CSI capability qualification remain pending. The caller must authorize each
operation and provide short repository mutations coupled to audit/outbox; a
database transaction must not span provider or restore I/O. The process
JSON/environment loader does not yet expose these adapters. Preparation tests
compose the real coordinator, Kubernetes client/provider, and mounted/layout
adapters using a local HTTP API fixture, an in-memory CAS repository, a fixture
generation restore, and a local filesystem. They do not qualify PostgreSQL,
actual Pod mounting, live-cluster storage, or CSI behavior.

For the inspected Raspberry Pi cluster and CSI backend assessment, see
[homelab storage](homelab-storage.md).
