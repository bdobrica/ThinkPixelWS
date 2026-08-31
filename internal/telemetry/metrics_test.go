package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestPrometheusRegistriesAreIsolated(t *testing.T) {
	first := NewPrometheusRegistry()
	second := NewPrometheusRegistry()

	for name, registry := range map[string]*prometheus.Registry{
		"first":  first,
		"second": second,
	} {
		families, err := registry.Gather()
		if err != nil {
			t.Fatalf("gather %s registry: %v", name, err)
		}
		foundGo := false
		foundProcess := false
		for _, family := range families {
			switch family.GetName() {
			case "go_goroutines":
				foundGo = true
			case "process_start_time_seconds":
				foundProcess = true
			}
		}
		if !foundGo || !foundProcess {
			t.Fatalf("%s registry missing runtime collectors: go=%t process=%t", name, foundGo, foundProcess)
		}
	}
}
