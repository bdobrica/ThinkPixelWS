package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	clockport "github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type Reader struct {
	Store    ports.WorkspaceReader
	Metadata ports.WorkspaceMetadataReader
	Cursors  *security.CursorCodec
	Clock    clockport.Clock
}

type Page struct {
	Items      []ports.WorkspaceRecord
	NextCursor string
}

func (r Reader) Get(ctx context.Context, tenant, id uuid.UUID) (ports.WorkspaceRecord, error) {
	if id.Version() != 7 || id.Variant() != uuid.RFC4122 {
		return ports.WorkspaceRecord{}, shared.NewError(shared.CodeInvalidArgument, "invalid Workspace ID")
	}
	if r.Store == nil {
		return ports.WorkspaceRecord{}, shared.NewError(shared.CodeUnavailable, "Workspace reads are unavailable")
	}
	result, err := r.Store.GetWorkspace(ctx, tenant, id)
	if errors.Is(err, ports.ErrWorkspaceNotFound) {
		return result, shared.NewError(shared.CodeNotFound, "Workspace not found")
	}
	if err != nil {
		return result, shared.WrapError(shared.CodeUnavailable, "Workspace read failed", err)
	}
	return result, nil
}

func (r Reader) List(ctx context.Context, tenant uuid.UUID, principal, cursor string, limit int) (Page, error) {
	if limit < 1 || limit > 500 {
		return Page{}, shared.NewError(shared.CodeInvalidArgument, "limit must be between 1 and 500")
	}
	if r.Store == nil || r.Cursors == nil || r.Clock == nil {
		return Page{}, shared.NewError(shared.CodeUnavailable, "Workspace reads are unavailable")
	}
	// Hash the structured scope to avoid ambiguous delimiters or identity leakage.
	raw, _ := json.Marshal([]string{"listWorkspaces", tenant.String(), principal})
	digest := sha256.Sum256(raw)
	scope := hex.EncodeToString(digest[:])
	var after uuid.UUID
	if cursor != "" {
		if err := r.Cursors.Decode(cursor, scope, &after); err != nil {
			return Page{}, err
		}
		if after.Version() != 7 || after.Variant() != uuid.RFC4122 {
			return Page{}, shared.NewError(shared.CodeInvalidArgument, "invalid cursor position")
		}
	}
	items, err := r.Store.ListWorkspaces(ctx, tenant, after, limit+1)
	if err != nil {
		return Page{}, shared.WrapError(shared.CodeUnavailable, "Workspace list failed", err)
	}
	page := Page{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor, err = r.Cursors.Encode(scope, items[limit-1].ID, r.Clock.Now().Add(15*time.Minute))
		if err != nil {
			return Page{}, err
		}
	}
	return page, nil
}
