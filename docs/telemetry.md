# Metrics and tracing

ThinkPixelWS initializes telemetry through `internal/telemetry` without using the
mutable Prometheus or OpenTelemetry package globals. `NewPrometheusRegistry`
returns a per-process registry containing Go runtime and process collectors.
Service collectors are registered on that registry and will be exposed by the
HTTP foundation added in ENG-008.

Service-owned metric names use the `thinkpixelws` namespace; standard Go and
process collector names retain their upstream names. Labels must come from a
reviewed, finite vocabulary such as lifecycle state, operation, or provider
kind. Tenant, Workspace, generation, component, Materialization, execution, Run,
request, trace, provider handle, target, filename, binding, and user-supplied
values must not be metric labels. Those identifiers belong in redacted logs or
traces when needed.

`InitializeTracing` creates an OpenTelemetry SDK tracer provider with parent-aware
ratio sampling and W3C Trace Context propagation. Baggage is deliberately not
propagated because arbitrary baggage can contain sensitive or high-cardinality
values. The initializer returns the provider and propagator explicitly for
dependency injection. A nil
exporter is the safe non-exporting default; an adapter may supply a vendor-neutral
OpenTelemetry span exporter, which is wrapped in the SDK batch processor. Callers
must invoke `Shutdown` during graceful shutdown so buffered spans are flushed.

Only bounded service name, version, and deployment environment attributes are
accepted as resource metadata. Traces must not include Workspace content,
sensitive filenames, bodies, bearer or cookie headers, credentials, key material,
signed URLs, profile handles, raw binding references, or high-cardinality input.
Sampling does not replace redaction: sensitive attributes must be excluded before
a span reaches a sampler or exporter.
