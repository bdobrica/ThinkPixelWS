#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

python3 scripts/validate_phase0.py
./scripts/check-openapi.sh
git diff --check
