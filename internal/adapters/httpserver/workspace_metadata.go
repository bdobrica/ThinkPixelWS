package httpserver

import (
	"context"
	"encoding/json"
	"net/url"

	api "github.com/bdobrica/ThinkPixelWS/api/openapi"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

func (h *workspaceHandler) metadataAccess(ctx context.Context, id uuid.UUID) (security.Identity, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return identity, shared.NewError(shared.CodeUnauthorized, "authentication required")
	}
	// Resolve the parent in the authenticated tenant before consulting policy or
	// looking up a child. Missing and cross-tenant parents have the same response.
	if _, err := h.reader.Get(ctx, identity.TenantID, id); err != nil {
		return identity, err
	}
	if err := AuthorizeWorkspace(ctx, ports.WorkspaceView, id); err != nil {
		return identity, shared.NewError(shared.CodeForbidden, "Workspace view is not permitted")
	}
	return identity, nil
}

func (h *workspaceHandler) GetComponent(ctx context.Context, p api.GetComponentParams) (*api.Component, error) {
	id := uuid.UUID(p.WorkspaceID)
	identity, err := h.metadataAccess(ctx, id)
	if err != nil {
		return nil, err
	}
	item, err := h.reader.GetComponent(ctx, identity.TenantID, id, uuid.UUID(p.ComponentID))
	if err != nil {
		return nil, err
	}
	response, err := componentResponse(item)
	return &response, err
}

func (h *workspaceHandler) GetGeneration(ctx context.Context, p api.GetGenerationParams) (*api.Generation, error) {
	id := uuid.UUID(p.WorkspaceID)
	identity, err := h.metadataAccess(ctx, id)
	if err != nil {
		return nil, err
	}
	item, err := h.reader.GetGeneration(ctx, identity.TenantID, id, p.Generation)
	if err != nil {
		return nil, err
	}
	response := generationResponse(item)
	return &response, nil
}

func metadataLimit(cursor api.OptString, limit api.OptInt) (int, error) {
	if cursor.Set && cursor.Value == "" {
		return 0, shared.NewError(shared.CodeInvalidArgument, "cursor must not be empty")
	}
	if limit.Set {
		return limit.Value, nil
	}
	return 100, nil
}

func (h *workspaceHandler) ListComponents(ctx context.Context, p api.ListComponentsParams) (*api.ListComponentsOKHeaders, error) {
	id := uuid.UUID(p.WorkspaceID)
	identity, err := h.metadataAccess(ctx, id)
	if err != nil {
		return nil, err
	}
	limit, err := metadataLimit(p.Cursor, p.Limit)
	if err != nil {
		return nil, err
	}
	items, next, err := h.reader.ListComponents(ctx, identity.TenantID, id, identity.Principal, p.Cursor.Value, limit)
	if err != nil {
		return nil, err
	}
	response := &api.ListComponentsOKHeaders{Response: make([]api.Component, 0, len(items))}
	for _, item := range items {
		value, err := componentResponse(item)
		if err != nil {
			return nil, err
		}
		response.Response = append(response.Response, value)
	}
	if next != "" {
		response.NextCursor = api.NewOptString(next)
	}
	return response, nil
}

func (h *workspaceHandler) ListGenerations(ctx context.Context, p api.ListGenerationsParams) (*api.ListGenerationsOKHeaders, error) {
	id := uuid.UUID(p.WorkspaceID)
	identity, err := h.metadataAccess(ctx, id)
	if err != nil {
		return nil, err
	}
	limit, err := metadataLimit(p.Cursor, p.Limit)
	if err != nil {
		return nil, err
	}
	items, next, err := h.reader.ListGenerations(ctx, identity.TenantID, id, identity.Principal, p.Cursor.Value, limit)
	if err != nil {
		return nil, err
	}
	response := &api.ListGenerationsOKHeaders{Response: make([]api.Generation, 0, len(items))}
	for _, item := range items {
		response.Response = append(response.Response, generationResponse(item))
	}
	if next != "" {
		response.NextCursor = api.NewOptString(next)
	}
	return response, nil
}

func componentResponse(c ports.ComponentRecord) (api.Component, error) {
	kind, _ := json.Marshal(c.Kind)
	result := api.Component{ID: api.UUID(c.ID), Name: api.Name(c.Name), Kind: kind, Path: c.CanonicalPath, Taints: c.Taints}
	if c.Classification != "" {
		result.Classification = api.NewOptClassification(api.Classification(c.Classification))
	}
	if c.Source != nil {
		ref, err := url.Parse(c.Source.Ref)
		if err != nil {
			return result, shared.NewError(shared.CodeUnavailable, "invalid stored source reference")
		}
		mode, _ := json.Marshal(c.Source.Mode)
		source := api.SourceBinding{Ref: *ref, Mode: mode}
		if c.Source.LastResolvedRevision != nil {
			source.Revision = api.NewOptString(*c.Source.LastResolvedRevision)
		}
		result.Source = api.NewOptSourceBinding(source)
	}
	return result, nil
}

func generationResponse(g domain.WorkspaceGeneration) api.Generation {
	durability, _ := json.Marshal(g.Durability)
	result := api.Generation{WorkspaceId: api.UUID(g.WorkspaceID), Number: int(g.Number), State: string(g.State), ManifestDigest: g.ManifestDigest.String(), Durability: durability, CreatedAt: g.CreatedAt}
	if g.ParentNumber != nil {
		result.ParentGeneration = api.NewOptInt(int(*g.ParentNumber))
	}
	return result
}
