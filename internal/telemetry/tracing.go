package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const maxResourceAttributeLength = 128

// TracingConfig contains bounded, non-secret resource metadata. Exporter
// endpoints and credentials belong to the adapter that constructs an exporter,
// not to this package.
type TracingConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	SampleRatio    float64
}

// Tracing owns the OpenTelemetry SDK objects initialized for one service.
// Provider and Propagator are returned explicitly so callers do not depend on
// mutable OpenTelemetry globals.
type Tracing struct {
	Provider   *sdktrace.TracerProvider
	Propagator propagation.TextMapPropagator
}

// InitializeTracing configures parent-aware ratio sampling and W3C Trace
// Context propagation. A nil exporter is a valid, non-exporting
// configuration. When supplied, spans are sent through a batching processor.
func InitializeTracing(ctx context.Context, cfg TracingConfig, exporter sdktrace.SpanExporter) (*Tracing, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	attributes := []resource.Option{
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	}
	if cfg.ServiceVersion != "" {
		attributes = append(attributes, resource.WithAttributes(semconv.ServiceVersion(cfg.ServiceVersion)))
	}
	if cfg.Environment != "" {
		attributes = append(attributes, resource.WithAttributes(semconv.DeploymentEnvironmentName(cfg.Environment)))
	}
	res, err := resource.New(ctx, attributes...)
	if err != nil {
		return nil, fmt.Errorf("initialize OpenTelemetry resource: %w", err)
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	}
	if exporter != nil {
		options = append(options, sdktrace.WithBatcher(exporter))
	}

	return &Tracing{
		Provider:   sdktrace.NewTracerProvider(options...),
		Propagator: propagation.TraceContext{},
	}, nil
}

// Shutdown flushes pending spans and releases exporter resources.
func (tracing *Tracing) Shutdown(ctx context.Context) error {
	if tracing == nil || tracing.Provider == nil {
		return nil
	}
	return tracing.Provider.Shutdown(ctx)
}

func (cfg TracingConfig) validate() error {
	var problems []error
	for name, value := range map[string]string{
		"service name":    cfg.ServiceName,
		"service version": cfg.ServiceVersion,
		"environment":     cfg.Environment,
	} {
		if strings.TrimSpace(value) != value {
			problems = append(problems, fmt.Errorf("%s must not have surrounding whitespace", name))
		}
		if len(value) > maxResourceAttributeLength {
			problems = append(problems, fmt.Errorf("%s must not exceed %d bytes", name, maxResourceAttributeLength))
		}
	}
	if cfg.ServiceName == "" {
		problems = append(problems, errors.New("service name is required"))
	}
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		problems = append(problems, errors.New("sample ratio must be between 0 and 1"))
	}
	return errors.Join(problems...)
}
