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

if ! grep -q 'github.com/ogen-go/ogen/validate' \
  api/openapi/oas_create_materialization_component_access_item_equal_gen.go; then
  patch --batch --forward -s -d api/openapi -p1 < scripts/ogen-v1.24.0.patch
fi
