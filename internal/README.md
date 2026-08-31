# Internal package boundaries

ThinkPixelWS keeps business rules independent from delivery mechanisms and providers:

- `domain/` contains Workspace concepts and invariants. It must not import application, port, adapter, transport, database, or provider-specific packages.
- `app/` coordinates use cases using domain types and interfaces exposed through `ports/`.
- `ports/` defines narrow replaceable integration seams in terms owned by ThinkPixelWS.
- `adapters/` implements ports for transports, persistence, infrastructure providers, policy engines, and optional ThinkPixel integrations.
- `security/` and `telemetry/` contain shared enforcement and observability support; they must not become alternate business-logic layers.

Dependencies point inward: adapters may depend on ports, application code, and domain code; application code may depend on ports and domain code; domain code remains provider-neutral. Cross-component adapters use versioned wire contracts and never another repository's `internal` packages or database.
