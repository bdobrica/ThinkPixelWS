// Package telemetry provides shared observability primitives.
package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

const Redacted = "[REDACTED]"

// Correlation contains the canonical identifiers used to join logs, traces,
// events, and audit records. Empty identifiers are omitted.
type Correlation struct {
	Tenant            string
	WorkspaceID       string
	Generation        string
	ComponentID       string
	MaterializationID string
	ExecutionID       string
	RunID             string
	Provider          string
	Target            string
	RequestID         string
	TraceID           string
}

// Attrs returns correlation fields using their canonical wire names.
func (c Correlation) Attrs() []slog.Attr {
	values := [...]struct{ key, value string }{
		{"tenant", c.Tenant},
		{"workspace_id", c.WorkspaceID},
		{"generation", c.Generation},
		{"component_id", c.ComponentID},
		{"materialization_id", c.MaterializationID},
		{"execution_id", c.ExecutionID},
		{"run_id", c.RunID},
		{"provider", c.Provider},
		{"target", c.Target},
		{"request_id", c.RequestID},
		{"trace_id", c.TraceID},
	}
	attrs := make([]slog.Attr, 0, len(values))
	for _, value := range values {
		if value.value != "" {
			attrs = append(attrs, slog.String(value.key, value.value))
		}
	}
	return attrs
}

// WithCorrelation attaches non-empty canonical correlation identifiers.
func WithCorrelation(logger *slog.Logger, correlation Correlation) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	attrs := correlation.Attrs()
	args := make([]any, len(attrs))
	for i := range attrs {
		args[i] = attrs[i]
	}
	return logger.With(args...)
}

// NewJSONLogger returns a JSON logger that recursively redacts sensitive
// fields before records reach the output handler.
func NewJSONLogger(writer io.Writer, level slog.Leveler) *slog.Logger {
	if writer == nil {
		writer = io.Discard
	}
	if level == nil {
		level = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	return slog.New(NewRedactingHandler(handler))
}

// NewRedactingHandler wraps a handler and recursively removes secret-bearing
// values from attributes added either to a logger or to an individual record.
func NewRedactingHandler(next slog.Handler) slog.Handler {
	if next == nil {
		panic("telemetry: nil logging handler")
	}
	return &redactingHandler{next: next}
}

type redactingHandler struct{ next slog.Handler }

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr, 0))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i := range attrs {
		redacted[i] = redactAttr(attrs[i], 0)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name)}
}

const maxRedactionDepth = 32

func redactAttr(attr slog.Attr, depth int) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if sensitiveKey(attr.Key) {
		return slog.String(attr.Key, Redacted)
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		for i := range group {
			group[i] = redactAttr(group[i], depth+1)
		}
		return slog.GroupAttrs(attr.Key, group...)
	}
	if attr.Value.Kind() == slog.KindAny {
		attr.Value = slog.AnyValue(redactAny(attr.Value.Any(), depth+1, make(map[visit]bool)))
	}
	return attr
}

var sensitiveFragments = [...]string{
	"authorization", "cookie", "set_cookie", "token", "secret", "password",
	"passwd", "credential", "api_key", "apikey", "private_key", "key_material",
	"signed_url", "profile_handle", "binding_ref", "request_body", "response_body",
	"body", "content", "kubeconfig",
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(normalized)
	for _, fragment := range sensitiveFragments {
		fragment = strings.ReplaceAll(fragment, "_", "")
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

type visit struct {
	typeOf  reflect.Type
	pointer uintptr
}

func redactAny(value any, depth int, seen map[visit]bool) any {
	if value == nil {
		return value
	}
	if depth > maxRedactionDepth {
		return Redacted
	}
	return redactReflect(reflect.ValueOf(value), depth, seen)
}

func redactReflect(value reflect.Value, depth int, seen map[visit]bool) any {
	if !value.IsValid() || depth > maxRedactionDepth {
		return Redacted
	}
	if value.CanInterface() {
		switch v := value.Interface().(type) {
		case time.Time, time.Duration:
			return v
		}
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		if value.Kind() == reflect.Pointer {
			marker := visit{value.Type(), value.Pointer()}
			if seen[marker] {
				return Redacted
			}
			seen[marker] = true
			defer delete(seen, marker)
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Struct:
		result := make(map[string]any, value.NumField())
		typeOf := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := typeOf.Field(i)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" {
				if tag == "-" {
					continue
				}
				name = tag
			}
			if sensitiveKey(name) {
				result[name] = Redacted
			} else {
				result[name] = redactReflect(value.Field(i), depth+1, seen)
			}
		}
		return result
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		marker := visit{value.Type(), value.Pointer()}
		if seen[marker] {
			return Redacted
		}
		seen[marker] = true
		defer delete(seen, marker)
		result := make(map[string]any, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			key := fmt.Sprint(iterator.Key().Interface())
			if sensitiveKey(key) {
				result[key] = Redacted
			} else {
				result[key] = redactReflect(iterator.Value(), depth+1, seen)
			}
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = redactReflect(value.Index(i), depth+1, seen)
		}
		return result
	default:
		if value.CanInterface() {
			return value.Interface()
		}
		return Redacted
	}
}
