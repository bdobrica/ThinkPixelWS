# OpenAPI development

[`contracts/openapi.yaml`](contracts/openapi.yaml) is the canonical public API
contract. Go transport code in `api/openapi/` is a derived artifact and MUST NOT
be edited by hand.

Generate the types after changing the contract:

```sh
./scripts/generate-openapi.sh
```

Validate the OpenAPI document and fail if committed generated types have drifted:

```sh
./scripts/check-openapi.sh
```

The OpenAPI 3.1-capable Go generator and toolchain are pinned in `go.mod`. The
independent Redocly validator is pinned in the check script. Generated client,
server, schema, and transport model code defines the wire boundary; application
behavior remains behind the service's explicit adapters and ports.

`ogen.yml` narrowly permits generation past unsupported OIDC security generation
and complex default-value generation. It does not weaken the contract: Redocly
still validates those declarations, OIDC enforcement remains an explicit HTTP
adapter concern scheduled under the IAM implementation items, and application
code remains responsible for documented request defaults.

The generation scripts also apply `scripts/ogen-v1.24.0.patch`, a pinned
workaround for ogen v1.24.0 omitting a validation import when generating the
contract's complex `uniqueItems` check. The patch fails closed if upstream output
changes and SHOULD be removed when the generator is upgraded to a fixed release.
