// Command thinkpixelws runs the Workspace Service control plane.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/adapters/httpserver"
	"github.com/bdobrica/ThinkPixelWS/internal/adapters/postgres"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/config"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/bdobrica/ThinkPixelWS/internal/telemetry"
	_ "github.com/lib/pq"
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

	var api http.Handler
	var readiness httpserver.Readiness
	if cfg.DatabaseURLFile != "" {
		db, err := openDatabase(cfg.DatabaseURLFile)
		if err != nil {
			return err
		}
		defer db.Close()
		readiness = databaseReadiness{db}
		cursors, cursorErr := cursorCodec(cfg.CursorKeyFile)
		if cursorErr != nil {
			return cursorErr
		}
		api, err = httpserver.NewWorkspaceAPI(workspace.Creator{Store: postgres.WorkspaceCreator{DB: db}, Clock: clockadapter.System{}}, workspace.Reader{Store: postgres.WorkspaceReader{DB: db}, Cursors: cursors, Clock: clockadapter.System{}})
		if err != nil {
			return fmt.Errorf("initialize Workspace API: %w", err)
		}
	}

	server, err := httpserver.New(cfg, httpserver.Dependencies{
		API:        api,
		Readiness:  readiness,
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

// The locator is configuration; the database credential stays outside Workspace state.
func openDatabase(path string) (*sql.DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read database URL file")
	}
	if len(data) == 0 || len(data) > 8192 || strings.TrimSpace(string(data)) == "" {
		return nil, errors.New("invalid database URL file")
	}
	db, err := sql.Open("postgres", strings.TrimSpace(string(data)))
	if err != nil {
		return nil, errors.New("cannot initialize PostgreSQL")
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("cannot connect to PostgreSQL")
	}
	return db, nil
}

type databaseReadiness struct{ db *sql.DB }

func (r databaseReadiness) Ready(ctx context.Context) error { return r.db.PingContext(ctx) }

// An optional external key keeps cursors valid across restarts/replicas.
// Without it, cursors deliberately last only for this process lifetime.
func cursorCodec(path string) (*security.CursorCodec, error) {
	key := make([]byte, 32)
	if path == "" {
		if _, err := rand.Read(key); err != nil {
			return nil, errors.New("generate cursor key failed")
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, errors.New("read cursor key failed")
		}
		defer file.Close()
		key, err = io.ReadAll(io.LimitReader(file, 33))
		if err != nil || len(key) != 32 {
			return nil, errors.New("cursor key file must contain exactly 32 raw bytes")
		}
	}
	return security.NewCursorCodec(key, clockadapter.System{})
}
