package postgres

import (
	"database/sql"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func TestWorkspaceWriterFenceRange(t *testing.T) {
	w := validStoredWorkspace(t)
	for _, fence := range []uint64{0, 1, math.MaxInt64} {
		w.WriterFence = fence
		if err := w.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	w.WriterFence = uint64(math.MaxInt64) + 1
	if err := w.Validate(); err == nil {
		t.Fatal("overflowing fence accepted")
	}
	repo := NewWorkspaceRepository(nil)
	for _, ids := range [][2]uuid.UUID{{uuid.Nil, w.ID}, {w.TenantID, uuid.Nil}} {
		if _, err := repo.AdvanceWriterFence(t.Context(), ids[0], ids[1]); err == nil {
			t.Fatal("missing scope accepted")
		}
	}
}

func TestWorkspaceWriterFencePostgres(t *testing.T) {
	db, dsn := materializationTestDatabase(t)
	ctx := t.Context()
	repo := NewWorkspaceRepository(db)
	w := validStoredWorkspace(t)
	w.CreatedAt = w.CreatedAt.UTC().Truncate(time.Microsecond)
	w.UpdatedAt = w.CreatedAt
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, w.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, w.TenantID, w); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][2]uuid.UUID{{uuid.Must(uuid.NewV7()), w.ID}, {w.TenantID, uuid.Must(uuid.NewV7())}} {
		if _, err := repo.AdvanceWriterFence(ctx, ids[0], ids[1]); !errors.Is(err, ports.ErrWorkspaceNotFound) {
			t.Fatalf("scope: %v", err)
		}
	}
	// Independent concurrent calls must each receive one distinct committed token.
	const writers = 16
	type result struct {
		fence uint64
		err   error
	}
	results := make(chan result, writers)
	start := make(chan struct{})
	for range writers {
		go func() { <-start; f, e := repo.AdvanceWriterFence(ctx, w.TenantID, w.ID); results <- result{f, e} }()
	}
	close(start)
	seen := map[uint64]bool{}
	for range writers {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.fence < 1 || r.fence > writers || seen[r.fence] {
			t.Fatalf("invalid or duplicate fence: %d", r.fence)
		}
		seen[r.fence] = true
	}
	reopened, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := NewWorkspaceRepository(reopened).Get(ctx, w.TenantID, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WriterFence != writers || got.StateVersion != w.StateVersion || got.State != w.State || !got.UpdatedAt.Equal(w.UpdatedAt) {
		t.Fatalf("unexpected persisted workspace: %#v", got)
	}
	// Lifecycle mutations must preserve the allocated fence.
	if err := repo.TransitionState(ctx, w.TenantID, w.ID, domain.WorkspaceCreating, domain.WorkspaceReady, 1, w.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	f, err := NewWorkspaceRepository(tx).AdvanceWriterFence(ctx, w.TenantID, w.ID)
	if err != nil || f != writers+1 {
		t.Fatalf("transaction fence: %d %v", f, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, w.TenantID, w.ID)
	if err != nil || got.WriterFence != writers {
		t.Fatalf("rollback: %#v %v", got, err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	f, err = NewWorkspaceRepository(tx).AdvanceWriterFence(ctx, w.TenantID, w.ID)
	if err != nil || f != writers+1 {
		t.Fatalf("committing fence: %d %v", f, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err = NewWorkspaceRepository(reopened).Get(ctx, w.TenantID, w.ID)
	if err != nil || got.WriterFence != writers+1 {
		t.Fatalf("committed fence not persisted: %#v %v", got, err)
	}
	// Another Workspace has its own sequence.
	other := w
	other.ID = uuid.Must(uuid.NewV7())
	other.Name = "other"
	if err := repo.Create(ctx, other.TenantID, other); err != nil {
		t.Fatal(err)
	}
	if f, err := repo.AdvanceWriterFence(ctx, other.TenantID, other.ID); err != nil || f != 1 {
		t.Fatalf("independent fence: %d %v", f, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE thinkpixelws.workspaces SET writer_fence=$3 WHERE tenant_id=$1 AND workspace_id=$2`, w.TenantID, w.ID, int64(math.MaxInt64-1)); err != nil {
		t.Fatal(err)
	}
	if f, err := repo.AdvanceWriterFence(ctx, w.TenantID, w.ID); err != nil || f != math.MaxInt64 {
		t.Fatalf("last fence: %d %v", f, err)
	}
	if f, err := repo.AdvanceWriterFence(ctx, w.TenantID, w.ID); !errors.Is(err, domain.ErrWorkspaceWriterFenceExhausted) || f != 0 {
		t.Fatalf("overflow: %d %v", f, err)
	}
	got, err = repo.Get(ctx, w.TenantID, w.ID)
	if err != nil || got.WriterFence != math.MaxInt64 {
		t.Fatalf("overflow changed fence: %#v %v", got, err)
	}
}
