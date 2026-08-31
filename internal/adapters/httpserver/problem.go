package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"go.opentelemetry.io/otel/trace"
)

// Problem is the service's RFC 7807 error representation.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	Code     string `json:"code,omitempty"`
	TraceID  string `json:"traceId,omitempty"`
}

// WriteProblem maps a transport-neutral error to an RFC 7807 response. Wrapped
// causes are deliberately not serialized.
func WriteProblem(w http.ResponseWriter, r *http.Request, err error) {
	code := shared.ErrorCodeOf(err)
	status, title := problemStatus(code)
	detail := http.StatusText(status)
	var typed *shared.Error
	if errors.As(err, &typed) && typed.Message != "" {
		detail = typed.Message
	}

	problem := Problem{
		Type:     "https://thinkpixel.io/problems/" + string(code),
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
		Code:     string(code),
	}
	if spanContext := trace.SpanContextFromContext(r.Context()); spanContext.IsValid() {
		problem.TraceID = spanContext.TraceID().String()
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}

func problemStatus(code shared.ErrorCode) (int, string) {
	switch code {
	case shared.CodeInvalidArgument:
		return http.StatusBadRequest, "Invalid request"
	case shared.CodeUnauthorized:
		return http.StatusUnauthorized, "Authentication required"
	case shared.CodeForbidden:
		return http.StatusForbidden, "Access denied"
	case shared.CodeNotFound:
		return http.StatusNotFound, "Resource not found"
	case shared.CodeConflict:
		return http.StatusConflict, "Conflict"
	case shared.CodeExpired:
		return http.StatusGone, "Resource expired"
	case shared.CodeTooLarge:
		return http.StatusRequestEntityTooLarge, "Request too large"
	case shared.CodeUnavailable:
		return http.StatusServiceUnavailable, "Service unavailable"
	default:
		return http.StatusInternalServerError, "Internal server error"
	}
}
