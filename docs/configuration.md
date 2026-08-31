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
