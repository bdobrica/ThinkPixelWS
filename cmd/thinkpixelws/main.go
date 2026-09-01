// Command thinkpixelws runs the Workspace Service control plane.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/adapters/httpserver"
	"github.com/bdobrica/ThinkPixelWS/internal/config"
	"github.com/bdobrica/ThinkPixelWS/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "thinkpixelws: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadFromEnvironment()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := telemetry.NewJSONLogger(os.Stdout, logLevel(cfg.Log.Level))
	tracing, err := telemetry.InitializeTracing(context.Background(), telemetry.TracingConfig{
		ServiceName: "thinkpixelws",
		SampleRatio: 0,
	}, nil)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}

	server, err := httpserver.New(cfg, httpserver.Dependencies{
		Registry:   telemetry.NewPrometheusRegistry(),
		Tracer:     tracing.Provider,
		Propagator: tracing.Propagator,
		Clock:      clockadapter.System{},
		Logger:     logger,
	})
	if err != nil {
		return fmt.Errorf("initialize HTTP server: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("service starting", "http_address", cfg.HTTP.ListenAddress, "metrics_address", cfg.Metrics.ListenAddress)
	serveErr := server.Serve(ctx)
	shutdownErr := tracing.Shutdown(context.Background())
	if serveErr != nil {
		return fmt.Errorf("serve: %w", serveErr)
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown tracing: %w", shutdownErr)
	}
	logger.Info("service stopped")
	return nil
}

func logLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
