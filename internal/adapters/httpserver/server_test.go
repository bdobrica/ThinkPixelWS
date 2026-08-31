package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/config"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type readinessFunc func(context.Context) error

func (fn readinessFunc) Ready(ctx context.Context) error { return fn(ctx) }

func dependencies(api http.Handler) Dependencies {
	return Dependencies{
		API: api, Registry: prometheus.NewRegistry(),
		Tracer:     sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample())),
		Propagator: propagation.TraceContext{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestPublicHandlerHealthRequestIDAndTraceContext(t *testing.T) {
	handler := newPublicHandler(config.Defaults().HTTP, dependencies(nil))
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if _, err := shared.ParseUUIDv7(response.Header().Get(RequestIDHeader)); err != nil {
		t.Fatalf("request ID is not UUIDv7: %v", err)
	}
	var problem Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace ID = %q", problem.TraceID)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("live response = %d %q", response.Code, response.Header().Get("Content-Type"))
	}
}

func TestPublicHandlerReadinessFailure(t *testing.T) {
	deps := dependencies(nil)
	deps.Readiness = readinessFunc(func(context.Context) error { return errors.New("database secret detail") })
	response := httptest.NewRecorder()
	newPublicHandler(config.Defaults().HTTP, deps).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "database secret detail") {
		t.Fatalf("readiness response = %d %q", response.Code, response.Body.String())
	}
}

func TestPublicHandlerLimitsAndDeadline(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Deadline(); !ok {
			t.Error("request has no deadline")
		}
		if RequestIDFromContext(r.Context()) == "" {
			t.Error("request ID missing from context")
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	})
	handler := newPublicHandler(config.Defaults().HTTP, dependencies(api))
	request := httptest.NewRequest(http.MethodPost, "/v1/test", bytes.NewReader(make([]byte, MaxBodyBytes+1)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	request.Header.Set("X-Large", strings.Repeat("x", MaxHeaderValueBytes+1))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large header status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/test", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("ordinary request status = %d", response.Code)
	}
}

func TestPublicHandlerRecoversPanic(t *testing.T) {
	handler := newPublicHandler(config.Defaults().HTTP, dependencies(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive panic")
	})))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/test", nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "sensitive panic") {
		t.Fatalf("panic response = %d %q", response.Code, response.Body.String())
	}
}

func TestNewRequiresExplicitDependencies(t *testing.T) {
	if _, err := New(config.Defaults(), Dependencies{}); err == nil {
		t.Fatal("New accepted missing dependencies")
	}
	server, err := New(config.Defaults(), dependencies(nil))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	response := httptest.NewRecorder()
	server.metrics.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", response.Code)
	}
}

func TestProblemDoesNotExposeWrappedCause(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	WriteProblem(response, request, shared.WrapError(shared.CodeConflict, "safe detail", errors.New("secret cause")))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "safe detail") || strings.Contains(response.Body.String(), "secret cause") {
		t.Fatalf("problem response = %d %q", response.Code, response.Body.String())
	}
}

func TestDeadlineCancels(t *testing.T) {
	cfg := config.Defaults().HTTP
	cfg.RequestTimeout = time.Millisecond
	done := make(chan struct{})
	handler := newPublicHandler(cfg, dependencies(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(done)
	})))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/test", nil))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request context was not cancelled")
	}
}
