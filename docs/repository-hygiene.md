# Repository hygiene

Git is source storage, not Workspace storage. Workspace snapshots, source
credentials, authenticated browser profiles, kubeconfigs, tokens, private keys,
and repository-local runtime or test data must remain outside this repository.
This preserves the credential and profile boundaries in ADR-0001, ADR-0004, and
ADR-0006.

The root `.gitignore` prevents common local artifacts from being added normally.
`make check-repository-hygiene` also examines every path and blob in the Git
index. It rejects sensitive path classes and high-confidence signatures for
private keys and common provider tokens. The tracked-file check matters because
ignore rules do not affect files already tracked or added with `git add -f`.

Sanitized, deterministic test fixtures may be committed under a package's
`testdata/` directory. They must contain no live or realistic credentials,
personal data, Workspace exports, browser state, or environment-specific data.
Use explicit synthetic placeholders such as `replace-me` where a fixture needs
to represent a secret reference.

This gate is deliberately not a general-purpose secret scanner and cannot prove
that arbitrary content is safe. Before committing, inspect staged changes and
use the organization's approved secret-scanning controls where available. If a
credential reaches Git, treat it as compromised: revoke or rotate it first, then
remove it from the repository and history using the incident-response process.
