#!/bin/sh
set -eu

workflow=.github/workflows/ci.yml

test -f "$workflow"
grep -Eq '^permissions:$' "$workflow"
grep -Eq '^  contents: read$' "$workflow"
grep -Eq '^    runs-on: ubuntu-[0-9]{2}\.[0-9]{2}$' "$workflow"
grep -Eq '^    timeout-minutes: [0-9]+$' "$workflow"
grep -Fq 'persist-credentials: false' "$workflow"
grep -Fq 'run: make verify' "$workflow"
grep -Fq 'run: docker build --tag thinkpixelws:ci .' "$workflow"

if grep -Eq '^[[:space:]]*pull_request_target:' "$workflow"; then
	echo 'CI must not run pull-request code with pull_request_target authority' >&2
	exit 1
fi

if grep -Eq '^[[:space:]]+[A-Za-z-]+: write([[:space:]]|$)' "$workflow"; then
	echo 'CI jobs must not request write permissions' >&2
	exit 1
fi

if grep -E '^[[:space:]]*-?[[:space:]]*uses:' "$workflow" | grep -Ev '@[0-9a-f]{40}([[:space:]]|$)'; then
	echo 'CI actions must be pinned to full commit SHAs' >&2
	exit 1
fi
