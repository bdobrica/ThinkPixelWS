# Command-line client

`thinkpixelwsctl` is a thin client for the public ThinkPixelWS API. It does not
read the service database or bypass server-side authentication, authorization,
tenant isolation, idempotency, or policy enforcement.

The Phase 1 skeleton provides read-only discovery commands:

```sh
go run ./cmd/thinkpixelwsctl --token-file /path/to/token workspace list
go run ./cmd/thinkpixelwsctl --token-file /path/to/token workspace describe WORKSPACE_ID
```

The API URL defaults to `http://127.0.0.1:8080`. Set it with `--server` or
`THINKPIXELWS_API_URL`. Plain HTTP is accepted only for loopback development
endpoints; other endpoints require HTTPS.

Supply an OIDC bearer token through `--token-file` or
`THINKPIXELWS_TOKEN_FILE`. The file must contain one non-empty token. There is
deliberately no token-value flag or token-value environment variable, which
reduces exposure through process arguments and inherited environment. Protect
the file using operating-system permissions and use a short-lived,
audience-bound token. The CLI sends the token only in the HTTP `Authorization`
header and writes API responses as JSON.

Run `go run ./cmd/thinkpixelwsctl help` for command help. Additional workflows
listed in `PLAN.md` remain scheduled for OPS-002.
