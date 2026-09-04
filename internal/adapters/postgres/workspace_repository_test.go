package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
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

func TestWorkspaceRepositoryTransitionStateUsesTenantScopedCompareAndSwap(t *testing.T) {
	t.Parallel()

	workspace := validStoredWorkspace(t)
	transitionedAt := workspace.UpdatedAt.Add(time.Second)
	db := &transitionDatabase{result: transitionResult{affected: 1}}
	repository := NewWorkspaceRepository(db)

	err := repository.TransitionState(t.Context(), workspace.TenantID, workspace.ID,
		domain.WorkspaceCreating, domain.WorkspaceReady, 1, transitionedAt)
	if err != nil {
		t.Fatalf("transition state: %v", err)
	}
	for _, predicate := range []string{"tenant_id = $1", "workspace_id = $2", "lifecycle_state = $3", "state_version = $4"} {
		if !strings.Contains(db.query, predicate) {
			t.Fatalf("transition query is missing compare-and-swap predicate %q", predicate)
		}
	}
	if len(db.arguments) != 6 || db.arguments[0] != workspace.TenantID || db.arguments[1] != workspace.ID || db.arguments[3] != uint64(1) {
		t.Fatalf("unexpected transition arguments: %#v", db.arguments)
	}
}

func TestWorkspaceRepositoryTransitionStateReportsConflict(t *testing.T) {
	t.Parallel()

	workspace := validStoredWorkspace(t)
	repository := NewWorkspaceRepository(&transitionDatabase{result: transitionResult{affected: 0}})
	err := repository.TransitionState(t.Context(), workspace.TenantID, workspace.ID,
		domain.WorkspaceCreating, domain.WorkspaceReady, 1, workspace.UpdatedAt)
	if !errors.Is(err, ports.ErrWorkspaceStateConflict) {
		t.Fatalf("expected state conflict, got %v", err)
	}
}

func TestWorkspaceRepositoryTransitionStateRejectsInvalidEdge(t *testing.T) {
	t.Parallel()

	workspace := validStoredWorkspace(t)
	repository := NewWorkspaceRepository(&transitionDatabase{})
	err := repository.TransitionState(t.Context(), workspace.TenantID, workspace.ID,
		domain.WorkspaceCreating, domain.WorkspaceArchived, 1, workspace.UpdatedAt)
	if !errors.Is(err, domain.ErrInvalidWorkspaceStateTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

type transitionDatabase struct {
	query     string
	arguments []any
	result    sql.Result
}

func (database *transitionDatabase) ExecContext(_ context.Context, query string, arguments ...any) (sql.Result, error) {
	database.query = query
	database.arguments = arguments
	return database.result, nil
}

func (*transitionDatabase) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

type transitionResult struct{ affected int64 }

func (transitionResult) LastInsertId() (int64, error)        { return 0, errors.New("unsupported") }
func (result transitionResult) RowsAffected() (int64, error) { return result.affected, nil }

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
