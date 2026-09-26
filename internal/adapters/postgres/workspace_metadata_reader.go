package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// The joins include every ownership key. Classification belongs to the current
// head; missing generation metadata is not replaced with invented defaults.
const componentReadSQL = `SELECT c.tenant_id,c.workspace_id,c.component_id,c.name,c.kind,c.canonical_path,c.created_at,
 s.provider,s.source_ref,s.mode,s.last_resolved_revision,s.created_at,
 COALESCE(m.classification,''),array_to_json(COALESCE(m.taints,'{}'::text[]))::text
 FROM thinkpixelws.workspace_components c
 JOIN thinkpixelws.workspaces w USING (tenant_id,workspace_id)
 LEFT JOIN thinkpixelws.source_bindings s USING (tenant_id,workspace_id,component_id)
 LEFT JOIN thinkpixelws.component_classification_metadata m
 ON m.tenant_id=c.tenant_id AND m.workspace_id=c.workspace_id AND m.component_id=c.component_id AND m.generation=w.head_generation
 WHERE c.tenant_id=$1 AND c.workspace_id=$2`

func (r WorkspaceReader) GetComponent(ctx context.Context, tenant, workspace, id uuid.UUID) (ports.ComponentRecord, error) {
	item, err := scanComponent(r.DB.QueryRowContext(ctx, componentReadSQL+` AND c.component_id=$3`, tenant, workspace, id))
	return item, metadataError(err)
}

func (r WorkspaceReader) ListComponents(ctx context.Context, tenant, workspace, after uuid.UUID, limit int) ([]ports.ComponentRecord, error) {
	if tenant == uuid.Nil || workspace == uuid.Nil || limit < 1 || limit > 501 {
		return nil, errors.New("invalid component list scope or limit")
	}
	rows, err := r.DB.QueryContext(ctx, componentReadSQL+` AND c.component_id>$3 ORDER BY c.component_id ASC LIMIT $4`, tenant, workspace, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ports.ComponentRecord, 0)
	for rows.Next() {
		item, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanComponent(row rowScanner) (ports.ComponentRecord, error) {
	var c ports.ComponentRecord
	var provider, ref, mode, revision sql.NullString
	var created sql.NullTime
	var taints string
	err := row.Scan(&c.TenantID, &c.WorkspaceID, &c.ID, &c.Name, &c.Kind, &c.CanonicalPath, &c.CreatedAt, &provider, &ref, &mode, &revision, &created, &c.Classification, &taints)
	if err != nil {
		return c, err
	}
	if err = c.Validate(); err != nil {
		return c, err
	}
	if err = json.Unmarshal([]byte(taints), &c.Taints); err != nil {
		return c, err
	}
	if ref.Valid {
		s := domain.SourceBinding{TenantID: c.TenantID, WorkspaceID: c.WorkspaceID, ComponentID: c.ID, Provider: provider.String, Ref: ref.String, Mode: domain.SourceBindingMode(mode.String), CreatedAt: created.Time}
		if revision.Valid {
			s.LastResolvedRevision = &revision.String
		}
		if err = s.Validate(); err != nil {
			return c, err
		}
		c.Source = &s
	}
	return c, nil
}

const generationReadSQL = `SELECT tenant_id,workspace_id,generation_id,generation,parent_generation,state,manifest_digest,durability,created_by_principal,created_by_execution_id,created_by_run_id,created_at,component_references
 FROM thinkpixelws.workspace_generations WHERE tenant_id=$1 AND workspace_id=$2`

func (r WorkspaceReader) GetGeneration(ctx context.Context, tenant, workspace uuid.UUID, number int) (domain.WorkspaceGeneration, error) {
	item, err := scanGeneration(r.DB.QueryRowContext(ctx, generationReadSQL+` AND generation=$3`, tenant, workspace, number))
	return item, metadataError(err)
}

func (r WorkspaceReader) ListGenerations(ctx context.Context, tenant, workspace uuid.UUID, after, limit int) ([]domain.WorkspaceGeneration, error) {
	if tenant == uuid.Nil || workspace == uuid.Nil || after < 0 || limit < 1 || limit > 501 {
		return nil, errors.New("invalid generation list scope or limit")
	}
	rows, err := r.DB.QueryContext(ctx, generationReadSQL+` AND generation>$3 ORDER BY generation ASC LIMIT $4`, tenant, workspace, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.WorkspaceGeneration, 0)
	for rows.Next() {
		item, err := scanGeneration(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanGeneration(row rowScanner) (domain.WorkspaceGeneration, error) {
	var g domain.WorkspaceGeneration
	var digest string
	var references []byte
	err := row.Scan(&g.TenantID, &g.WorkspaceID, &g.ID, &g.Number, &g.ParentNumber, &g.State, &digest, &g.Durability, &g.CreatedByPrincipal, &g.CreatedByExecution, &g.CreatedByRun, &g.CreatedAt, &references)
	if err != nil {
		return g, err
	}
	if references != nil {
		if err := json.Unmarshal(references, &g.ComponentReferences); err != nil {
			return g, err
		}
	}
	g.ManifestDigest, err = shared.ParseSHA256Digest(digest)
	if err != nil {
		return g, err
	}
	return g, g.Validate()
}

func metadataError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ErrWorkspaceMetadataNotFound
	}
	return err
}

var _ ports.WorkspaceMetadataReader = WorkspaceReader{}
