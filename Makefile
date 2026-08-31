.DEFAULT_GOAL := help

GO ?= go
GO_PACKAGES ?= ./...
STATICCHECK_VERSION ?= v0.7.0
GOVULNCHECK_VERSION ?= v1.7.0
GO_LICENSES_VERSION ?= v2.0.1
APPROVED_LICENSES := Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MIT-0,Unicode-3.0,Zlib

.PHONY: help generate generate-openapi check check-openapi format vet lint unit race vulnerability license build validate validate-phase0 verify

help:
	@printf '%s\n' \
		'ThinkPixelWS development targets:' \
		'  make generate        Regenerate all committed derived artifacts' \
		'  make check           Run fast source and contract checks' \
		'  make verify          Run the aggregate repository verification gate' \
		'  make format          Check Go source formatting' \
		'  make vet             Run Go vet' \
		'  make lint            Run pinned Staticcheck' \
		'  make unit            Run unit tests' \
		'  make race            Run unit tests with the race detector' \
		'  make vulnerability   Scan reachable code with pinned govulncheck' \
		'  make license         Enforce the dependency license allowlist' \
		'  make build           Build all Go packages' \
		'  make generate-openapi Regenerate the OpenAPI Go package' \
		'  make check-openapi    Validate OpenAPI and detect generated-code drift' \
		'  make validate-phase0  Validate Phase 0 schemas, documentation, and contracts'

generate: generate-openapi

generate-openapi:
	./scripts/generate-openapi.sh

check: format vet lint check-openapi

check-openapi:
	./scripts/check-openapi.sh

format:
	@test -z "$$(gofmt -l $$(git ls-files -- '*.go'))" || { gofmt -d $$(git ls-files -- '*.go'); echo 'Go files are not formatted.' >&2; exit 1; }

vet:
	$(GO) vet $(GO_PACKAGES)

lint:
	$(GO) run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) $(GO_PACKAGES)

unit:
	$(GO) test $(GO_PACKAGES)

race:
	$(GO) test -race $(GO_PACKAGES)

vulnerability:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) -test $(GO_PACKAGES)

license:
	GO='$(GO)' GO_LICENSES_VERSION='$(GO_LICENSES_VERSION)' APPROVED_LICENSES='$(APPROVED_LICENSES)' ./scripts/check-licenses.sh

build:
	$(GO) build $(GO_PACKAGES)

validate: validate-phase0

verify: check unit race vulnerability license build validate

validate-phase0:
	./scripts/validate-phase0.sh
