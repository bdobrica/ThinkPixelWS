# ADR-0001: Core boundaries and authority

- Status: Accepted
- Date: 2026-08-30

## Context

Durable work, disposable compute, agent interaction continuity, and permission are separate concerns. Collapsing them would make storage identity infrastructure-specific and let contextual references become ambient authority.

## Decision

ThinkPixelWS owns durable work context and metadata. A Workspace is neither a Materialization nor a ThinkPixelAR Session. A WorkspaceBinding describes context and is never an AccessGrant. ThinkPixelAG owns integrated execution authority; ThinkPixelAR owns Sessions, Executions, Attempts, sandboxes, and scheduling; ThinkPixelTG owns governed external side effects and downstream credentials.

Workspace content MUST NOT intentionally contain platform execution credentials. Source import MUST NOT imply source write-back. Public Workspace identity MUST NOT contain a PVC, node, cluster, VM, Session, or sandbox identifier.

## Consequences

Administrative authorization and execution authorization are independently evaluated. Provider handles remain opaque. External bindings require a separate grant at use time. Standalone deployments must supply an explicit local authorizer and may not silently bypass authorization.

## Alternatives considered

- Make AR canonical for Workspace state: rejected because Session/compute loss would affect work identity.
- Treat membership as authorization: rejected because it creates confused-deputy and stale-authority risks.
- Put source credentials in Materializations: rejected because snapshots and hostile workloads could retain them.

## Verification

The OpenAPI grant fields, integration contracts, threat model, and future authorization tests enforce the separation.
