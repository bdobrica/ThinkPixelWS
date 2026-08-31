# HTTP server foundation

The baseline delivery adapter is `internal/adapters/httpserver`. It owns two
explicitly configured listeners:

- the public listener serves `/livez`, `/readyz`, and the injected API handler;
- the administrative metrics listener serves `/metrics` from the injected,
  process-local Prometheus registry.

Health endpoints do not require authentication. `/livez` reports process
liveness. `/readyz` calls an injected readiness dependency and returns `503`
without exposing the dependency failure when the service cannot accept work.
The metrics listener defaults to loopback and must be protected by deployment
network policy when configured on a non-loopback interface.

Every public request receives a server-generated UUIDv7 `X-Request-ID`. Client
values are not trusted or reused. The adapter extracts only W3C Trace Context,
starts a server span using the explicitly injected tracer provider, and makes
the request ID available through `RequestIDFromContext`.

The adapter applies the ADR-0007 ordinary-request defaults: a 30-second context
deadline, a 1 MiB body reader limit, an 8 KiB limit for each header value, and
the configured 64 KiB aggregate header limit. Streaming import/export handlers
must use their separate limits. API handlers must handle body read/decode errors;
`http.MaxBytesError` means the streamed body exceeded the enforced limit.

Panics are recovered at the transport boundary and produce a redacted RFC 7807
internal error. Typed domain errors map to stable HTTP status, problem `code`,
and safe `detail` fields; wrapped internal causes are never serialized. The
server gracefully drains both listeners when its context is cancelled or when
either listener fails. The caller remains responsible for shutting down its
OpenTelemetry provider within the same process shutdown sequence.
