package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
	reader  workspace.Reader
}

// NewWorkspaceAPI uses the published contract's generated decoder and router.
// The enclosing Server supplies authentication; every operation requires policy.
func NewWorkspaceAPI(creator workspace.Creator, reader workspace.Reader) (http.Handler, error) {
	handler := &workspaceHandler{creator: creator, reader: reader}
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
		isCreate := r.Method == "POST" && r.URL.Path == "/v1/workspaces"
		isRead := r.Method == "GET" && (r.URL.Path == "/v1/workspaces" || (strings.HasPrefix(r.URL.Path, "/v1/workspaces/") && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/v1/workspaces/"), "/")))
		if !isCreate && !isRead {
			WriteProblem(w, r, shared.NewError(shared.CodeNotFound, "endpoint is not implemented"))
			return
		}
		if isCreate && len(r.Header.Values("Idempotency-Key")) != 1 {
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
	return &api.WorkspaceHeaders{Location: api.NewOptString("/v1/workspaces/" + result.ID.String()), Response: workspaceResponse(ports.WorkspaceRecord{Workspace: result})}, nil
}

func workspaceResponse(result ports.WorkspaceRecord) api.Workspace {
	ownerKind, _ := json.Marshal(result.Owner.Kind)
	state, _ := json.Marshal(result.State)
	class, _ := json.Marshal(result.Classification)
	response := api.Workspace{
		ID: api.UUID(result.ID), Name: api.Name(result.Name),
		Owner: api.Owner{Kind: ownerKind, ID: result.Owner.ID},
		State: state, StateVersion: int(result.StateVersion),
		Classification: class, Residency: result.Residency, CreatedAt: result.CreatedAt,
	}
	if result.HeadGeneration > 0 {
		response.HeadGeneration = api.NewOptInt(int(result.HeadGeneration))
	}
	return response
}

func (h *workspaceHandler) GetWorkspace(ctx context.Context, params api.GetWorkspaceParams) (*api.Workspace, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return nil, shared.NewError(shared.CodeUnauthorized, "authentication required")
	}
	result, err := h.reader.Get(ctx, identity.TenantID, uuid.UUID(params.WorkspaceID))
	if err != nil {
		return nil, err
	}
	if err := AuthorizeWorkspace(ctx, ports.WorkspaceView, result.ID); err != nil {
		return nil, shared.NewError(shared.CodeForbidden, "Workspace view is not permitted")
	}
	response := workspaceResponse(result)
	return &response, nil
}

func (h *workspaceHandler) ListWorkspaces(ctx context.Context, params api.ListWorkspacesParams) (*api.WorkspacePage, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return nil, shared.NewError(shared.CodeUnauthorized, "authentication required")
	}
	if err := AuthorizeWorkspace(ctx, ports.WorkspaceList, uuid.Nil); err != nil {
		return nil, shared.NewError(shared.CodeForbidden, "Workspace listing is not permitted")
	}
	if params.Cursor.Set && params.Cursor.Value == "" {
		return nil, shared.NewError(shared.CodeInvalidArgument, "cursor must not be empty")
	}
	limit := 100
	if params.Limit.Set {
		limit = params.Limit.Value
	}
	page, err := h.reader.List(ctx, identity.TenantID, identity.Principal, params.Cursor.Value, limit)
	if err != nil {
		return nil, err
	}
	response := &api.WorkspacePage{Items: make([]api.Workspace, 0, len(page.Items))}
	for _, item := range page.Items {
		// Reevaluate view policy for each returned resource on every page.
		if err := AuthorizeWorkspace(ctx, ports.WorkspaceView, item.ID); err != nil {
			continue
		}
		response.Items = append(response.Items, workspaceResponse(item))
	}
	if page.NextCursor != "" {
		response.NextCursor = api.NewOptNilString(page.NextCursor)
	}
	return response, nil
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
