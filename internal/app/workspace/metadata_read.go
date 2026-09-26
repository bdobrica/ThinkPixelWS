package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// Metadata operations are called after the enclosing Workspace has been found
// within the authenticated tenant and authorized for view by the API handler.
func (r Reader) GetComponent(ctx context.Context, tenant, workspace, id uuid.UUID) (ports.ComponentRecord, error) {
	if id.Version() != 7 || id.Variant() != uuid.RFC4122 {
		return ports.ComponentRecord{}, shared.NewError(shared.CodeInvalidArgument, "invalid component ID")
	}
	if r.Metadata == nil {
		return ports.ComponentRecord{}, metadataReadError(nil)
	}
	item, err := r.Metadata.GetComponent(ctx, tenant, workspace, id)
	if err != nil {
		return item, metadataReadError(err)
	}
	return item, nil
}

func (r Reader) GetGeneration(ctx context.Context, tenant, workspace uuid.UUID, number int) (domain.WorkspaceGeneration, error) {
	if number < 1 {
		return domain.WorkspaceGeneration{}, shared.NewError(shared.CodeInvalidArgument, "invalid generation number")
	}
	if r.Metadata == nil {
		return domain.WorkspaceGeneration{}, metadataReadError(nil)
	}
	item, err := r.Metadata.GetGeneration(ctx, tenant, workspace, number)
	if err != nil {
		return item, metadataReadError(err)
	}
	return item, nil
}

func metadataReadError(err error) error {
	if errors.Is(err, ports.ErrWorkspaceMetadataNotFound) {
		return shared.NewError(shared.CodeNotFound, "Workspace metadata not found")
	}
	return shared.WrapError(shared.CodeUnavailable, "Workspace metadata read unavailable", err)
}

func (r Reader) metadataCursor(operation string, tenant, workspace uuid.UUID, principal, cursor string, limit int, after any) (string, error) {
	if limit < 1 || limit > 500 {
		return "", shared.NewError(shared.CodeInvalidArgument, "limit must be between 1 and 500")
	}
	if r.Metadata == nil || r.Cursors == nil || r.Clock == nil {
		return "", metadataReadError(nil)
	}
	raw, _ := json.Marshal([]string{operation, tenant.String(), workspace.String(), principal})
	digest := sha256.Sum256(raw)
	scope := hex.EncodeToString(digest[:])
	if cursor != "" {
		if err := r.Cursors.Decode(cursor, scope, after); err != nil {
			return "", err
		}
	}
	return scope, nil
}

func (r Reader) ListComponents(ctx context.Context, tenant, workspace uuid.UUID, principal, cursor string, limit int) ([]ports.ComponentRecord, string, error) {
	var after uuid.UUID
	scope, err := r.metadataCursor("listComponents", tenant, workspace, principal, cursor, limit, &after)
	if err != nil {
		return nil, "", err
	}
	if cursor != "" && (after.Version() != 7 || after.Variant() != uuid.RFC4122) {
		return nil, "", shared.NewError(shared.CodeInvalidArgument, "invalid cursor position")
	}
	items, err := r.Metadata.ListComponents(ctx, tenant, workspace, after, limit+1)
	if err != nil {
		return nil, "", metadataReadError(err)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next, err = r.Cursors.Encode(scope, items[limit-1].ID, r.Clock.Now().Add(15*time.Minute))
	}
	return items, next, err
}

func (r Reader) ListGenerations(ctx context.Context, tenant, workspace uuid.UUID, principal, cursor string, limit int) ([]domain.WorkspaceGeneration, string, error) {
	var after int
	scope, err := r.metadataCursor("listGenerations", tenant, workspace, principal, cursor, limit, &after)
	if err != nil {
		return nil, "", err
	}
	if cursor != "" && after < 1 {
		return nil, "", shared.NewError(shared.CodeInvalidArgument, "invalid cursor position")
	}
	items, err := r.Metadata.ListGenerations(ctx, tenant, workspace, after, limit+1)
	if err != nil {
		return nil, "", metadataReadError(err)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next, err = r.Cursors.Encode(scope, items[limit-1].Number, r.Clock.Now().Add(15*time.Minute))
	}
	return items, next, err
}
