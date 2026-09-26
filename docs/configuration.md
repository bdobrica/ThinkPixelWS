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
remain unauthenticated. Workspace creation additionally requires PostgreSQL
configuration below; list/get are not implemented yet.

`disabled` disables development auth; it does not enable anonymous API access.
Configured API handlers return 503 until authentication is configured. There is
no production OIDC HTTP mode yet. Unknown modes and identity/token settings with
`disabled` are rejected rather than silently ignored.


## Workspace API and PostgreSQL

`POST /v1/workspaces` is available when `THINKPIXELWS_DATABASE_URL_FILE` points
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

The focused PostgreSQL integration test creates and drops an isolated database.
Point it at a disposable local instance using a URL for an account with CREATEDB:

```sh
THINKPIXELWS_TEST_DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/postgres?sslmode=disable' \
  go test -race ./internal/adapters/httpserver -run TestCreateWorkspacePostgres -v
```
