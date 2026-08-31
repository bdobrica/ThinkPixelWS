# ThinkPixelWS documentation

This directory contains the normative Phase 0 architecture baseline.

- `architecture.md` defines system context, trust boundaries, vocabulary, invariants, state, concurrency, composition, durability, and lifecycle.
- `security.md` defines the threat model, content-handling limits, credential/profile rules, authorization boundaries, and observability redaction.
- `operations.md` defines placement, residency, retention, events, SLO assumptions, and supported infrastructure.
- `database-model.md` defines the authoritative PostgreSQL model and invariants.
- `enterprise-integration.md` defines ThinkPixelAG, ThinkPixelAR, ThinkPixelTG, ThinkPixelMP, ThinkPixelMEM, and optional ThinkPixelGR boundaries.
- `portable-snapshot-evaluation.md` records the portable-format decision and benchmark plan.
- `supported-versions.md` is the compatibility policy.
- `dependency-policy.md` defines dependency selection, approved sources, integrity, license classification, and exception requirements.
- `configuration.md` defines typed process configuration, precedence, defaults, validation, and secret-reference handling.
- `logging.md` defines canonical structured-log correlation fields and recursive redaction behavior.
- `telemetry.md` defines isolated Prometheus registration, bounded metric labels, and OpenTelemetry initialization.
- `adr/` contains accepted architecture decisions.
- `contracts/` contains machine-readable and provider-facing contracts.
- `phase-0-evidence.md` records validation evidence and remaining environment-dependent verification.

Normative words such as MUST, MUST NOT, SHOULD, and MAY are interpreted as described by RFC 2119/RFC 8174.
