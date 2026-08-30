#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

python3 scripts/validate_phase0.py
npx --yes @redocly/cli@2.12.2 lint docs/contracts/openapi.yaml
git diff --check
