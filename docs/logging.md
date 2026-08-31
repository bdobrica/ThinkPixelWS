# Structured logging

ThinkPixelWS emits structured JSON logs through the standard Go `slog` API. The shared handler in `internal/telemetry` performs recursive redaction before a record reaches the JSON handler. Code should attach identifiers through `telemetry.Correlation` instead of inventing field names.

Canonical correlation fields are `tenant`, `workspace_id`, `generation`, `component_id`, `materialization_id`, `execution_id`, `run_id`, `provider`, `target`, `request_id`, and `trace_id`. Empty identifiers are omitted. These values are correlation context only and do not establish authority.

The handler recursively redacts secret-bearing keys in groups, maps, slices, arrays, and exported struct fields, including authorization and cookie data, tokens, passwords, credentials, key material, signed URLs, profile handles, binding references, request or response bodies, content, and kubeconfigs. Attributes attached with `Logger.With` receive the same treatment as per-record attributes. Cyclic and excessively deep values fail closed.

Redaction is a last-resort safety control, not permission to log sensitive inputs. Callers must not submit Workspace file contents, sensitive filenames, request or response bodies, source credentials, key material, profile state, raw external bindings, or high-cardinality user input. Metrics and tracing exporters must apply their own bounded-label and redaction rules.
