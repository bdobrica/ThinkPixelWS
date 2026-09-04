package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

func TestWorkspaceRepositoryRejectsMismatchedTenantScope(t *testing.T) {
	t.Parallel()

	repository := NewWorkspaceRepository(nil)
	workspace := validStoredWorkspace(t)
	otherTenant, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(t.Context(), otherTenant, workspace); err == nil {
		t.Fatal("expected mismatched tenant scope to be rejected")
	}
}

func TestScanWorkspace(t *testing.T) {
	t.Parallel()

	want := validStoredWorkspace(t)
	row := valueRow{values: []any{
		want.TenantID, want.ID, want.Name, want.Description,
		want.Owner.Kind, want.Owner.ID, want.State, want.StateVersion,
		want.WriterFence, want.Classification, []byte(`["region:ro"]`),
		want.CreatedAt, want.UpdatedAt,
	}}

	got, err := scanWorkspace(row)
	if err != nil {
		t.Fatalf("scan workspace: %v", err)
	}
	if got.TenantID != want.TenantID || got.ID != want.ID || len(got.Residency) != 1 || got.Residency[0] != "region:ro" {
		t.Fatalf("unexpected scanned workspace: %#v", got)
	}
}

type valueRow struct {
	values []any
}

func (row valueRow) Scan(destinations ...any) error {
	if len(destinations) != len(row.values) {
		return errors.New("destination count does not match values")
	}
	for index, destination := range destinations {
		switch pointer := destination.(type) {
		case *uuid.UUID:
			*pointer = row.values[index].(uuid.UUID)
		case *string:
			*pointer = row.values[index].(string)
		case *domain.OwnerKind:
			*pointer = row.values[index].(domain.OwnerKind)
		case *domain.WorkspaceState:
			*pointer = row.values[index].(domain.WorkspaceState)
		case *uint64:
			*pointer = row.values[index].(uint64)
		case *domain.Classification:
			*pointer = row.values[index].(domain.Classification)
		case *[]byte:
			*pointer = row.values[index].([]byte)
		case *time.Time:
			*pointer = row.values[index].(time.Time)
		default:
			return errors.New("unsupported destination")
		}
	}
	return nil
}

func validStoredWorkspace(t *testing.T) domain.Workspace {
	t.Helper()
	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := (domain.NewWorkspace{
		TenantID:  tenantID,
		ID:        workspaceID,
		Name:      "workspace",
		Owner:     domain.Owner{Kind: domain.OwnerTeam, ID: "team"},
		Residency: []string{"region:ro"},
	}).Workspace(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}
