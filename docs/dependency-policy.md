# Dependency, source, and license policy

This policy applies to runtime, build, test, code-generation, container-image, and
deployment dependencies committed to or distributed by ThinkPixelWS. It also
applies to transitive dependencies. The repository license remains authoritative
for ThinkPixelWS code; a dependency's license applies only to that dependency.

## Selection

Dependencies MUST have a concrete repository-local use and MUST preserve the
ports/adapters boundaries described in `internal/README.md`. Prefer the Go
standard library and existing dependencies when they meet the requirement. A
new dependency MUST be actively maintained, have identifiable upstream source
and releases, and avoid requiring credentials or network access at runtime unless
that access is part of an explicit adapter.

The change introducing a dependency MUST record its purpose in the commit or
pull-request description. Security-sensitive dependencies (authentication,
authorization, cryptography, archive processing, parsing, persistence, or network
clients) require an explicit security review. Dependencies MUST NOT broaden Run
authority or move credentials into Workspace content or untrusted execution
state.

## Approved sources and integrity

- Go modules MUST use canonical module paths, be recorded in `go.mod`/`go.sum`,
  and pass the public Go checksum database through the configured Go proxy.
- A direct VCS fallback, private module, fork, local `replace`, vendored tree, or
  checksum-database exclusion requires a documented reason, owner, integrity
  mechanism, and removal or review condition. These exceptions MUST NOT be
  committed merely to make an unavailable upstream build pass.
- Container images and release artifacts MUST come from an identified publisher.
  Deployment and release inputs MUST be pinned by immutable digest; mutable tags
  MAY accompany a digest for readability but are not an integrity control.
- Downloaded tools, binaries, schemas, and generated inputs MUST be version-pinned
  and checksum- or signature-verified. Generated output MUST identify its source
  and reproducible generation command.
- Credentials MUST NOT appear in dependency URLs, module configuration, lock
  files, build arguments, or committed package-manager configuration.

Committed lock and checksum files are reviewed source artifacts. Updates SHOULD
be narrow and MUST NOT silently replace an upstream project with a different
publisher or fork.

## License classification

Every direct and transitive dependency distributed with the service, CLI,
container image, or deployment artifact MUST have an identifiable license and
retain its required copyright, license, and notice text.

The following SPDX licenses are approved without a separate legal exception when
their standard terms are unmodified:

- `Apache-2.0`
- `BSD-2-Clause`
- `BSD-3-Clause`
- `ISC`
- `MIT`
- `MIT-0`
- `Unicode-3.0`
- `Zlib`

The following require documented legal review before introduction because their
obligations depend on linkage, modification, distribution, or specific terms:

- `MPL-2.0`, `EPL-2.0`, `CDDL-1.0`, LGPL-family licenses, and other weak-copyleft
  licenses;
- dual- or multi-licensed packages when an approved option is not clearly
  selected;
- public-domain dedications, custom licenses, license exceptions, or packages
  whose metadata and source notices disagree.

The following are prohibited without a written legal exception approved before
merge:

- GPL-family and AGPL-family licenses;
- SSPL, Business Source License, Commons Clause, non-commercial, field-of-use,
  source-available, or other non-open-source restrictions;
- unlicensed code or artifacts, and dependencies with unknown licenses.

An exception MUST identify the exact component and version, intended use and
distribution, approving owner, obligations, expiry or review date, and replacement
plan. Approval of one version does not approve another. Exception records belong
in `docs/dependency-exceptions/`; the absence of that directory means there are
no approved exceptions.

## Review and verification

Dependency changes MUST include regenerated lock/checksum data and pass formatting,
tests, vulnerability analysis, license analysis, and build verification provided
by the repository aggregate verification target. Findings are evaluated for both
reachability and deployed artifacts; suppressions MUST be narrow, time-bounded,
owned, and documented.

Known critical or high vulnerabilities in shipped code MUST be fixed, removed, or
covered by an explicitly approved time-bounded risk acceptance before release.
End-of-life dependencies MUST NOT be introduced. Dependency and base-image updates
SHOULD be reviewed regularly and expedited for relevant security fixes.

Release artifacts MUST include dependency attribution and an SBOM. The repository
verification gate enforces the approved dependency-license list and scans reachable
Go code for known vulnerabilities. Artifact attribution and SBOM production remain
release-packaging responsibilities.
