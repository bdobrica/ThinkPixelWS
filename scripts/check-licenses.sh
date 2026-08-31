#!/usr/bin/env bash
set -euo pipefail

go_command="${GO:-go}"
go_licenses_version="${GO_LICENSES_VERSION:-v2.0.1}"
approved_licenses="${APPROVED_LICENSES:-Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MIT-0,Unicode-3.0,Zlib}"

# go-licenses v2.0.1 cannot associate segmentio/asm's assembly-backed
# subpackages with the MIT-0 license at the module root. Keep this workaround
# fail-closed by pinning both the module version and its license-file digest.
read -r asm_version asm_dir < <("${go_command}" list -m -f '{{.Version}} {{.Dir}}' github.com/segmentio/asm)
if [[ "${asm_version}" != "v1.2.1" ]]; then
	echo "unsupported github.com/segmentio/asm license exception version: ${asm_version}" >&2
	exit 1
fi

expected_asm_license_digest="cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443"
actual_asm_license_digest="$(sha256sum "${asm_dir}/LICENSE" | awk '{print $1}')"
if [[ "${actual_asm_license_digest}" != "${expected_asm_license_digest}" ]]; then
	echo 'github.com/segmentio/asm license text changed; review it before updating the exception.' >&2
	exit 1
fi

"${go_command}" run "github.com/google/go-licenses/v2@${go_licenses_version}" check \
	--include_tests \
	--allowed_licenses="${approved_licenses}" \
	--ignore=github.com/segmentio/asm \
	./...
