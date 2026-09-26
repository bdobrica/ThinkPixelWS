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
remain unauthenticated. Workspace API endpoints are separate implementation work;
starting the service currently exposes only the health and metrics routes.

`disabled` disables development auth; it does not enable anonymous API access.
Configured API handlers return 503 until authentication is configured. There is
no production OIDC HTTP mode yet. Unknown modes and identity/token settings with
`disabled` are rejected rather than silently ignored.
