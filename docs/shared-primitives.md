# Shared service primitives

The engineering foundation provides narrow, provider-neutral primitives for
identity, validation, time, errors, integrity, and pagination:

- `internal/domain/shared.UUIDv7` accepts and generates only canonical RFC 9562
  UUIDv7 identifiers. Generation uses an injected clock and cryptographic
  randomness.
- `internal/ports/clock.Clock` is the application-facing time source;
  `internal/adapters/clock.System` is the UTC production adapter. Tests and
  services can inject deterministic clocks.
- `internal/domain/shared.Error` carries a stable code, a caller-safe message,
  and an optional wrapped internal cause. Delivery adapters map these codes to
  transport-specific responses without exposing causes.
- `internal/domain/shared.BoundedString` validates Unicode UTF-8, requires NFC,
  rejects control characters, and measures bounds in Unicode code points. The
  direct `golang.org/x/text` dependency supplies the maintained Unicode NFC
  implementation.
- `internal/domain/shared.SHA256Digest` parses and emits only canonical
  `sha256:<64 lowercase hex>` identities.
- `internal/security.CursorCodec` produces URL-safe, HMAC-SHA-256-authenticated
  cursors with a version, expiry, bounded JSON payload, and caller-defined
  scope. Scope must include the list operation and relevant tenant, principal,
  filters, and ordering so a valid cursor cannot be replayed in another query.

Cursor payloads are opaque to API clients but are authenticated, not encrypted.
They must contain only non-secret pagination state. Cursor keys must be at least
32 bytes, come from secret configuration, and support deployment-managed
rotation before old keys are retired.
