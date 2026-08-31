#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

GOTOOLCHAIN=${GOTOOLCHAIN:-go1.25.14}
export GOTOOLCHAIN

go tool ogen \
  --target api/openapi \
  --package openapi \
  --clean \
  docs/contracts/openapi.yaml

patch -s -d api/openapi -p1 < scripts/ogen-v1.24.0.patch
