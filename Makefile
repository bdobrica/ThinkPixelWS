.PHONY: verify validate-phase0

verify: validate-phase0

validate-phase0:
	./scripts/validate-phase0.sh
