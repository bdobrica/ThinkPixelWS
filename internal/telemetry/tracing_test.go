package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestInitializeTracingPropagatesW3CContext(t *testing.T) {
	tracing, err := InitializeTracing(context.Background(), TracingConfig{
		ServiceName: "thinkpixelws",
		SampleRatio: 1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tracing.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracing: %v", err)
		}
	})

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	carrier := propagation.MapCarrier{}
	tracing.Propagator.Inject(trace.ContextWithRemoteSpanContext(context.Background(), spanContext), carrier)
	if carrier.Get("traceparent") == "" {
		t.Fatal("traceparent was not injected")
	}
	extracted := trace.SpanContextFromContext(tracing.Propagator.Extract(context.Background(), carrier))
	if !extracted.IsRemote() || extracted.TraceID() != spanContext.TraceID() || extracted.SpanID() != spanContext.SpanID() {
		t.Fatalf("unexpected extracted span context: %v", extracted)
	}
}

func TestInitializeTracingExportsSampledSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracing, err := InitializeTracing(context.Background(), TracingConfig{
		ServiceName:    "thinkpixelws",
		ServiceVersion: "test",
		Environment:    "unit",
		SampleRatio:    1,
	}, exporter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tracing.Shutdown(context.Background()) })

	_, span := tracing.Provider.Tracer("test").Start(context.Background(), "workspace.create")
	span.End()
	if err := tracing.Provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush spans: %v", err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one exported span, got %d", len(spans))
	}
	attributes := make(map[attribute.Key]string)
	for _, item := range spans[0].Resource.Attributes() {
		attributes[item.Key] = item.Value.AsString()
	}
	if attributes["service.name"] != "thinkpixelws" || attributes["service.version"] != "test" || attributes["deployment.environment.name"] != "unit" {
		t.Fatalf("unexpected resource attributes: %#v", attributes)
	}
}

func TestInitializeTracingValidatesBoundedResourceData(t *testing.T) {
	tests := []TracingConfig{
		{},
		{ServiceName: " thinkpixelws", SampleRatio: 1},
		{ServiceName: "thinkpixelws", SampleRatio: -0.1},
		{ServiceName: "thinkpixelws", SampleRatio: 1.1},
	}
	for _, cfg := range tests {
		if tracing, err := InitializeTracing(context.Background(), cfg, nil); err == nil {
			_ = tracing.Shutdown(context.Background())
			t.Fatalf("expected validation error for %#v", cfg)
		}
	}
}

func TestTracingShutdownAllowsNilReceiver(t *testing.T) {
	var tracing *Tracing
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
