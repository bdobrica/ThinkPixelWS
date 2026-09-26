# ThinkPixelWS

ThinkPixelWS is an open-source, vendor-neutral Workspace Service for durable, portable, and roaming AI-agent work contexts.

> **Work persists. Compute moves.**

A Workspace is durable logical context, independent of any Pod, VM, PVC, cluster, cloud, agent harness, or Session. It can contain multi-repository work, documents, generated artifacts, source provenance, and references to external resources or reproducible environments. Immutable generations can be materialized into disposable execution environments, checkpointed, committed, forked, archived, and restored.

The primary security invariant is:

> **Workspace membership describes context. It does not grant runtime authority.**

ThinkPixelWS keeps runtime authority, source-system credentials, model access, agent execution, long-term memory, and software qualification outside the Workspace boundary. The repository's precise role in the platform is documented in the [normative architecture](docs/architecture.md).

## Status

The normative Phase 0 architecture, security model, ADRs, provider contracts, OpenAPI 3.1 contract, and machine-readable Workspace/portable-snapshot schemas are complete. The engineering foundation is complete and durable Workspace implementation is underway; [`TODO.md`](TODO.md) tracks remaining implementation work.

An OIDC verifier adapter validates RS256 bearer JWTs against a configured issuer and audience, including expiry and signing-key rotation. Verified claims map to a tenant and principal using explicitly configured claim names. A typed administrative authorization port and fail-closed enforcement helper are available; an opt-in, loopback-only development mode supplies bearer authentication and a local create/list/view policy. OPA and production OIDC HTTP integration remain pending. See the [authentication boundary](docs/security.md#authentication-and-authorization).

For current cross-repository priority, follow the ThinkPixel platform [development alignment](https://github.com/bdobrica/ThinkPixel/blob/main/docs/development/ALIGNMENT.md). The immediate WS objective is the smallest durable Workspace path needed to prove that work survives disposable AR compute: materialize work, modify/checkpoint or commit it, destroy the sandbox, attach or recreate fresh compute, and continue with the same logical Workspace without expanding Run authority.

Portable cross-target roaming, forks, broad provider qualification, and production hardening remain important WS capabilities, but they do not block the first integrated ThinkPixel demo/RC unless the active platform alignment requires them.

## Key concepts

- **Workspace** — the long-lived, tenant-scoped logical unit of work.
- **WorkspaceGeneration** — an immutable committed Workspace state.
- **Materialization** — a disposable provider realization of one generation.
- **Checkpoint** — provider-local recovery state; not necessarily portable or committed.
- **PortableSnapshot** — encrypted, provider-independent committed content used for archive, recovery, and roaming.
- **Fork** — a new Workspace identity derived from an immutable generation.

See the [normative architecture](docs/architecture.md) and [documentation index](docs/README.md) for the complete model.

## Development

Requirements are Go 1.25.14, Python 3, Node.js/npm, and Git. The Go module
also declares the exact supported toolchain for automatic selection by the Go command.

Use focused checks while developing, then the aggregate repository gate when proportionate to the change and before declaring a phase or RC complete:

```sh
make help
make check
make verify
```

`make verify` runs formatting, vet, lint, unit, race, vulnerability, license,
build, and contract validation. CI runs the same gate with pinned, read-only
automation; the gate also rejects tracked credentials and local
Workspace/profile state. A failure unrelated to the active demo path should be
reported rather than automatically expanding a focused task into unrelated
hardening work.
See the [continuous-integration guide](docs/continuous-integration.md) and
[repository-hygiene guide](docs/repository-hygiene.md). See
[`PLAN.md`](PLAN.md) for implementation intent and
[`docs/phase-0-evidence.md`](docs/phase-0-evidence.md) and
[`docs/phase-1-evidence.md`](docs/phase-1-evidence.md) for phase evidence.

`POST /v1/workspaces` creates tenant-scoped Workspace metadata in PostgreSQL with authorization, durable idempotency, and atomic audit/outbox records. `GET /v1/workspaces` lists authorized metadata with tenant-safe cursor pagination; `GET /v1/workspaces/{workspace_id}` returns current metadata and the committed head when present. See [local API setup](docs/configuration.md#workspace-api-and-postgresql). Content/materialization APIs remain pending.

Local PostgreSQL development additionally requires Docker with Compose. Use
`make postgres-up`, then explicitly apply migrations with `make migrate`. See
the [database development guide](docs/database-development.md) for configuration
and lifecycle details.

The `thinkpixelwsctl` skeleton calls the public API for Workspace discovery.
See the [command-line client guide](docs/cli.md) for its commands and secure
token-file configuration.

The repository-root Makefile is the stable developer entry point. Use
`make generate` after changing generated inputs, `make check` for fast source and
contract checks, and `make verify` for the aggregate repository gate. Run
`make help` for the independently runnable checks.

Build the hardened, non-root service container with `make image`. Runtime
configuration must explicitly bind the public and metrics listeners to container
interfaces; see the [service image guide](docs/service-image.md).

## Documentation

- [Platform role and ownership boundary](docs/architecture.md)
- [Implementation plan](PLAN.md)
- [Release-candidate ledger](TODO.md)
- [Architecture](docs/architecture.md)
- [Security model](docs/security.md)
- [Enterprise integration contracts](docs/enterprise-integration.md)
- [API and schemas](docs/contracts/)
- [Accepted architecture decisions](docs/adr/)
- [Operations](docs/operations.md)
- [Supported versions and compatibility targets](docs/supported-versions.md)

## ThinkPixel platform

ThinkPixelWS is the Agent Runtime component of the broader [ThinkPixel](https://github.com/bdobrica/ThinkPixel) platform.

Platform architecture, component responsibilities, integration status, and cross-repository development priorities are maintained centrally:

- [ThinkPixel overview](https://github.com/bdobrica/ThinkPixel)
- [Platform development guidance](https://github.com/bdobrica/ThinkPixel/tree/main/docs/development)
- [Current development alignment](https://github.com/bdobrica/ThinkPixel/blob/main/docs/development/ALIGNMENT.md)

WS remains independently usable. ThinkPixel integrations are implemented through explicit adapters and versioned contracts rather than by importing other components' internal state or implementation details.

## License

Licensed under the terms in [LICENSE](LICENSE).
