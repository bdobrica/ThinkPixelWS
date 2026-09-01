#!/bin/sh
set -eu

dockerfile=${1:-Dockerfile}

grep -Eq '^FROM golang:1\.25\.14-trixie@sha256:[0-9a-f]{64} AS build$' "$dockerfile"
grep -Eq '^FROM gcr\.io/distroless/static-debian13:nonroot@sha256:[0-9a-f]{64}$' "$dockerfile"
grep -Eq '^USER 65532:65532$' "$dockerfile"
grep -Fq 'CGO_ENABLED=0 go build -trimpath' "$dockerfile"
grep -Fq 'ENTRYPOINT ["/usr/local/bin/thinkpixelws"]' "$dockerfile"

if grep -Eq '^FROM .*(latest|:nonroot)$' "$dockerfile"; then
	echo 'container stages must use immutable digest pins' >&2
	exit 1
fi
