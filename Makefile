.DEFAULT_GOAL := help

.PHONY: help generate generate-openapi check check-openapi validate validate-phase0 verify

help:
	@printf '%s\n' \
		'ThinkPixelWS development targets:' \
		'  make generate        Regenerate all committed derived artifacts' \
		'  make check           Run fast source and contract checks' \
		'  make verify          Run the aggregate repository verification gate' \
		'  make generate-openapi Regenerate the OpenAPI Go package' \
		'  make check-openapi    Validate OpenAPI and detect generated-code drift' \
		'  make validate-phase0  Validate Phase 0 schemas, documentation, and contracts'

generate: generate-openapi

generate-openapi:
	./scripts/generate-openapi.sh

check: check-openapi

check-openapi:
	./scripts/check-openapi.sh

validate: validate-phase0

verify: validate

validate-phase0:
	./scripts/validate-phase0.sh
