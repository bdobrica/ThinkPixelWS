package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	api "github.com/bdobrica/ThinkPixelWS/api/openapi"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type workspaceHandler struct {
	api.UnimplementedHandler
	creator workspace.Creator
}

// NewWorkspaceAPI uses the published contract's generated decoder and router.
// The enclosing Server supplies authentication; creation also requires policy.
func NewWorkspaceAPI(creator workspace.Creator) (http.Handler, error) {
	handler := &workspaceHandler{creator: creator}
	server, err := api.NewServer(handler, api.WithErrorHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
		code := shared.CodeInvalidArgument
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			code = shared.CodeTooLarge
		}
		WriteProblem(w, r, shared.NewError(code, "invalid API request"))
	}))
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/workspaces" {
			WriteProblem(w, r, shared.NewError(shared.CodeNotFound, "endpoint is not implemented"))
			return
		}
		if len(r.Header.Values("Idempotency-Key")) != 1 {
			WriteProblem(w, r, shared.NewError(shared.CodeInvalidArgument, "one Idempotency-Key is required"))
			return
		}
		server.ServeHTTP(w, r)
	}), nil
}

func (h *workspaceHandler) CreateWorkspace(ctx context.Context, req *api.CreateWorkspace, params api.CreateWorkspaceParams) (*api.WorkspaceHeaders, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return nil, shared.NewError(shared.CodeUnauthorized, "authentication required")
	}
	if err := AuthorizeWorkspace(ctx, ports.WorkspaceCreate, uuid.Nil); err != nil {
		return nil, shared.NewError(shared.CodeForbidden, "Workspace creation is not permitted")
	}
	var kind domain.OwnerKind
	var classification domain.Classification
	if err := json.Unmarshal(req.Owner.Kind, &kind); err != nil {
		return nil, shared.NewError(shared.CodeInvalidArgument, "invalid owner kind")
	}
	if len(req.Classification) > 0 {
		if err := json.Unmarshal(req.Classification, &classification); err != nil || classification == "" {
			return nil, shared.NewError(shared.CodeInvalidArgument, "invalid classification")
		}
	}
	traceID := ""
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		traceID = span.TraceID().String()
	}
	result, err := h.creator.Create(ctx, identity.TenantID, identity.Principal, params.IdempotencyKey, workspace.CreateInput{
		Name: string(req.Name), Owner: domain.Owner{Kind: kind, ID: req.Owner.ID},
		Classification: classification, Residency: req.Residency,
	}, RequestIDFromContext(ctx), traceID)
	if err != nil {
		return nil, err
	}
	ownerKind, _ := json.Marshal(result.Owner.Kind)
	state, _ := json.Marshal(result.State)
	class, _ := json.Marshal(result.Classification)
	return &api.WorkspaceHeaders{
		Location: api.NewOptString("/v1/workspaces/" + result.ID.String()),
		Response: api.Workspace{
			ID: api.UUID(result.ID), Name: api.Name(result.Name),
			Owner: api.Owner{Kind: ownerKind, ID: result.Owner.ID},
			State: state, StateVersion: int(result.StateVersion),
			Classification: class, Residency: result.Residency, CreatedAt: result.CreatedAt,
		},
	}, nil
}

func (h *workspaceHandler) NewError(ctx context.Context, err error) *api.ProblemStatusCode {
	code := shared.ErrorCodeOf(err)
	status, title := problemStatus(code)
	detail := http.StatusText(status)
	var typed *shared.Error
	if errors.As(err, &typed) {
		detail = typed.Message
	}
	problem := api.Problem{Type: "https://thinkpixel.io/problems/" + string(code), Title: title, Status: status, Detail: api.NewOptString(detail), Code: api.NewOptString(string(code))}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		problem.TraceId = api.NewOptString(span.TraceID().String())
	}
	return &api.ProblemStatusCode{StatusCode: status, Response: problem}
}
