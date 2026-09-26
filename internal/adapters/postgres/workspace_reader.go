package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

type WorkspaceReader struct{ DB *sql.DB }

const workspaceReadColumns = `tenant_id, workspace_id, name, COALESCE(description, ''), owner_kind, owner_id,
 lifecycle_state, state_version, writer_fence, classification,
 array_to_json(residency)::text, created_at, updated_at, COALESCE(head_generation, 0)`

func (r WorkspaceReader) GetWorkspace(ctx context.Context, tenant, id uuid.UUID) (ports.WorkspaceRecord, error) {
	result, err := scanWorkspaceRecord(r.DB.QueryRowContext(ctx, `SELECT `+workspaceReadColumns+`
 FROM thinkpixelws.workspaces WHERE tenant_id=$1 AND workspace_id=$2`, tenant, id))
	if errors.Is(err, sql.ErrNoRows) {
		return result, ports.ErrWorkspaceNotFound
	}
	return result, err
}

func (r WorkspaceReader) ListWorkspaces(ctx context.Context, tenant, after uuid.UUID, limit int) ([]ports.WorkspaceRecord, error) {
	if tenant == uuid.Nil || limit < 1 || limit > 501 {
		return nil, errors.New("invalid workspace list scope or limit")
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+workspaceReadColumns+`
 FROM thinkpixelws.workspaces WHERE tenant_id=$1 AND workspace_id>$2
 ORDER BY workspace_id ASC LIMIT $3`, tenant, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ports.WorkspaceRecord, 0)
	for rows.Next() {
		item, err := scanWorkspaceRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type headRow struct {
	rowScanner
	head *uint64
}

func (r headRow) Scan(dest ...any) error { return r.rowScanner.Scan(append(dest, r.head)...) }
func scanWorkspaceRecord(row rowScanner) (ports.WorkspaceRecord, error) {
	var result ports.WorkspaceRecord
	var err error
	result.Workspace, err = scanWorkspace(headRow{row, &result.HeadGeneration})
	return result, err
}

var _ ports.WorkspaceReader = WorkspaceReader{}
