# Continuous integration

GitHub Actions runs the repository's existing `make verify` gate for pull
requests and pushes to `main`. A separate job builds the hardened service image,
keeping Docker-specific work isolated from source verification.

The workflow applies these supply-chain and least-privilege controls:

- workflow token permissions are limited to read-only repository contents;
- third-party action references are pinned to full commit SHAs, with release
  versions retained in comments for reviewability;
- checkout credentials are not persisted in the working tree;
- jobs use an explicitly selected Ubuntu runner image and finite timeouts;
- pull requests use `pull_request`, not the more privileged
  `pull_request_target` event;
- redundant runs on the same ref are cancelled without sharing artifacts or
  credentials between jobs.

GitHub-hosted runner images cannot be selected by immutable digest. The explicit
`ubuntu-24.04` label avoids the moving `ubuntu-latest` alias; action and container
dependencies remain immutably pinned where the platform supports it.

Run `make check-ci` locally to validate the workflow's security invariants. Run
`make verify` to execute the same source gate used by CI. The image build remains
a distinct CI job because it requires a Docker daemon.
