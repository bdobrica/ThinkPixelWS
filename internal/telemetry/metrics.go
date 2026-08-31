package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NewPrometheusRegistry returns an isolated registry with the standard Go
// runtime and process collectors. Callers register service-specific collectors
// on this registry and expose it through promhttp when the HTTP server starts.
//
// An isolated registry avoids the mutable package-global default registry and
// makes duplicate service/test initialization deterministic.
func NewPrometheusRegistry() *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return registry
}
