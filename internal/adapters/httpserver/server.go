// Package httpserver provides the baseline HTTP delivery adapter.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/config"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	clockport "github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	MaxBodyBytes        int64 = 1 << 20
	MaxHeaderValueBytes       = 8 << 10
	RequestIDHeader           = "X-Request-ID"
)

// Readiness reports whether the service can accept ordinary traffic.
type Readiness interface {
	Ready(context.Context) error
}

type Dependencies struct {
	API        http.Handler
	Registry   *prometheus.Registry
	Tracer     trace.TracerProvider
	Propagator propagation.TextMapPropagator
	Clock      clockport.Clock
	Readiness  Readiness
	Logger     *slog.Logger
}

// Server owns the public and administrative HTTP listeners.
type Server struct {
	public          *http.Server
	metrics         *http.Server
	shutdownTimeout time.Duration
}

type requestIDContextKey struct{}

// RequestIDFromContext returns the server-generated request correlation ID.
func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func New(cfg config.Config, deps Dependencies) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate HTTP server configuration: %w", err)
	}
	if deps.Registry == nil || deps.Tracer == nil || deps.Propagator == nil || deps.Clock == nil {
		return nil, errors.New("registry, tracer, propagator, and clock are required")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	publicHandler := newPublicHandler(cfg.HTTP, deps)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.HandlerFor(deps.Registry, promhttp.HandlerOpts{}))

	return &Server{
		public: &http.Server{
			Addr:              cfg.HTTP.ListenAddress,
			Handler:           publicHandler,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.RequestTimeout + cfg.HTTP.ReadHeaderTimeout,
			WriteTimeout:      cfg.HTTP.RequestTimeout + time.Second,
			IdleTimeout:       2 * cfg.HTTP.RequestTimeout,
			MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
		},
		metrics: &http.Server{
			Addr:              cfg.Metrics.ListenAddress,
			Handler:           metricsMux,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.RequestTimeout,
			WriteTimeout:      cfg.HTTP.RequestTimeout,
			IdleTimeout:       2 * cfg.HTTP.RequestTimeout,
			MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
		},
		shutdownTimeout: cfg.HTTP.ShutdownTimeout,
	}, nil
}

// Serve listens until ctx is cancelled or either listener fails. Both
// listeners are then gracefully drained within the configured shutdown limit.
func (s *Server) Serve(ctx context.Context) error {
	publicListener, err := net.Listen("tcp", s.public.Addr)
	if err != nil {
		return fmt.Errorf("listen on public address: %w", err)
	}
	metricsListener, err := net.Listen("tcp", s.metrics.Addr)
	if err != nil {
		_ = publicListener.Close()
		return fmt.Errorf("listen on metrics address: %w", err)
	}

	errCh := make(chan error, 2)
	go serve(s.public, publicListener, errCh)
	go serve(s.metrics, metricsListener, errCh)

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	var wg sync.WaitGroup
	shutdownErrors := make(chan error, 2)
	for _, server := range []*http.Server{s.public, s.metrics} {
		wg.Add(1)
		go func(server *http.Server) {
			defer wg.Done()
			shutdownErrors <- server.Shutdown(shutdownCtx)
		}(server)
	}
	wg.Wait()
	close(shutdownErrors)
	for shutdownErr := range shutdownErrors {
		serveErr = errors.Join(serveErr, shutdownErr)
	}
	return serveErr
}

func serve(server *http.Server, listener net.Listener, result chan<- error) {
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		result <- err
	}
}

func newPublicHandler(cfg config.HTTPConfig, deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { writeStatus(w, http.StatusOK, "live") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if deps.Readiness != nil {
			if err := deps.Readiness.Ready(r.Context()); err != nil {
				WriteProblem(w, r, shared.NewError(shared.CodeUnavailable, "service is not ready"))
				return
			}
		}
		writeStatus(w, http.StatusOK, "ready")
	})
	if deps.API != nil {
		mux.Handle("/", deps.API)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			WriteProblem(w, r, shared.NewError(shared.CodeNotFound, "route not found"))
		})
	}

	var handler http.Handler = recoverPanics(deps.Logger, mux)
	handler = requestDeadline(cfg.RequestTimeout, handler)
	handler = requestLimits(handler)
	handler = tracing(deps.Tracer, deps.Propagator, handler)
	handler = requestID(deps.Clock, handler)
	return handler
}

func writeStatus(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}

func requestDeadline(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > MaxBodyBytes {
			WriteProblem(w, r, shared.NewError(shared.CodeTooLarge, "request body exceeds 1048576 bytes"))
			return
		}
		for name, values := range r.Header {
			for _, value := range values {
				if len(name)+len(value) > MaxHeaderValueBytes {
					WriteProblem(w, r, shared.NewError(shared.CodeTooLarge, "individual header exceeds 8192 bytes"))
					return
				}
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func requestID(clock clockport.Clock, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := shared.NewUUIDv7(clock)
		if err != nil {
			WriteProblem(w, r, shared.NewError(shared.CodeInternal, "could not create request identifier"))
			return
		}
		w.Header().Set(RequestIDHeader, id.String())
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, id.String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tracing(provider trace.TracerProvider, propagator propagation.TextMapPropagator, next http.Handler) http.Handler {
	tracer := provider.Tracer("github.com/bdobrica/ThinkPixelWS/internal/adapters/httpserver")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		// Method is bounded; raw URL paths may contain sensitive or
		// high-cardinality identifiers and are deliberately excluded.
		ctx, span := tracer.Start(ctx, "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(r.Context(), "recovered HTTP panic", "request_id", RequestIDFromContext(r.Context()))
				WriteProblem(w, r, shared.NewError(shared.CodeInternal, "request failed"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
