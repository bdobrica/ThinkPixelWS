#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

npx --yes @redocly/cli@2.12.2 lint docs/contracts/openapi.yaml

generated=$(mktemp -d)
trap 'rm -rf "$generated"' EXIT HUP INT TERM

GOTOOLCHAIN=${GOTOOLCHAIN:-go1.25.14}
export GOTOOLCHAIN

go tool ogen \
  --target "$generated" \
  --package openapi \
  --clean \
  docs/contracts/openapi.yaml

if ! grep -q 'github.com/ogen-go/ogen/validate' \
  "$generated/oas_create_materialization_component_access_item_equal_gen.go"; then
  patch --batch --forward -s -d "$generated" -p1 < scripts/ogen-v1.24.0.patch
fi

if ! diff -ru --exclude=.gitkeep api/openapi "$generated" >/dev/null; then
  echo "openapi: generated Go code is stale; run ./scripts/generate-openapi.sh" >&2
  diff -ru --exclude=.gitkeep api/openapi "$generated" || true
  exit 1
fi

echo "openapi: contract is valid and generated Go code is current"
