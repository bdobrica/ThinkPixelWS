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

func writerGuardFixture(t *testing.T) (*sql.DB, *MaterializationLeaseRepository, domain.Materialization, domain.MaterializationLease, ports.MaterializationWriter) {
	t.Helper()
	db, repo, m, l := leaseLifecycleFixture(t)
	for _, query := range []string{
		`UPDATE thinkpixelws.workspaces SET head_generation=1 WHERE tenant_id=$1 AND workspace_id=$2`,
		`UPDATE thinkpixelws.materializations SET lifecycle_state='ACTIVE' WHERE tenant_id=$1 AND workspace_id=$2`,
	} {
		if _, err := db.ExecContext(t.Context(), query, m.TenantID, m.WorkspaceID); err != nil {
			t.Fatal(err)
		}
	}
	return db, repo, m, l, ports.MaterializationWriter{TenantID: m.TenantID, WorkspaceID: m.WorkspaceID, MaterializationID: m.ID, LeaseID: l.ID, Fence: l.FencingToken}
}

func checkWriter(t *testing.T, db *sql.DB, w ports.MaterializationWriter, commit bool, head uint64) error {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	guard := NewMaterializationWriterGuard(tx)
	if commit {
		return guard.ValidateCommit(t.Context(), w, head)
	}
	return guard.ValidateCheckpoint(t.Context(), w)
}

func TestMaterializationWriterGuardPostgres(t *testing.T) {
	db, _, m, l, w := writerGuardFixture(t)
	for _, commit := range []bool{false, true} {
		if err := checkWriter(t, db, w, commit, 1); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"tenant", "workspace", "materialization", "lease", "fence", "zero", "overflow"} {
			bad := w
			switch field {
			case "tenant":
				bad.TenantID = uuid.Must(uuid.NewV7())
			case "workspace":
				bad.WorkspaceID = uuid.Must(uuid.NewV7())
			case "materialization":
				bad.MaterializationID = uuid.Must(uuid.NewV7())
			case "lease":
				bad.LeaseID = uuid.Must(uuid.NewV7())
			case "fence":
				bad.Fence++
			case "zero":
				bad.Fence = 0
			case "overflow":
				bad.Fence = math.MaxUint64
			}
			if err := checkWriter(t, db, bad, commit, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
				t.Fatalf("commit=%v %s: %v", commit, field, err)
			}
		}
	}
	for _, head := range []uint64{0, 2, math.MaxUint64} {
		if err := checkWriter(t, db, w, true, head); !errors.Is(err, ports.ErrWorkspaceHeadConflict) {
			t.Fatalf("head %d: %v", head, err)
		}
	}
	for _, state := range []domain.MaterializationState{domain.MaterializationRequested, domain.MaterializationPreparing, domain.MaterializationReady, domain.MaterializationReleasing, domain.MaterializationReleased, domain.MaterializationFailed, domain.MaterializationFenced, domain.MaterializationCheckpointing} {
		if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.materializations SET lifecycle_state=$3 WHERE tenant_id=$1 AND materialization_id=$2`, m.TenantID, m.ID, state); err != nil {
			t.Fatal(err)
		}
		for _, commit := range []bool{false, true} {
			err := checkWriter(t, db, w, commit, 1)
			if state == domain.MaterializationCheckpointing {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ports.ErrMaterializationWriterConflict) {
				t.Fatalf("%s accepted: %v", state, err)
			}
		}
	}
	ageLease(t, db, l)
	for _, commit := range []bool{false, true} {
		if err := checkWriter(t, db, w, commit, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
			t.Fatalf("expired accepted: %v", err)
		}
	}
}

func TestMaterializationWriterGuardTakeoverPostgres(t *testing.T) {
	db, repo, m, l, w := writerGuardFixture(t)
	ageLease(t, db, l)
	replacement := m
	replacement.ID = uuid.Must(uuid.NewV7())
	if err := NewMaterializationRepository(db).Create(t.Context(), m.TenantID, replacement); err != nil {
		t.Fatal(err)
	}
	next, err := repo.Acquire(t.Context(), m.TenantID, replacement.ID, uuid.Must(uuid.NewV7()), "replacement")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.materializations SET lifecycle_state='ACTIVE' WHERE tenant_id=$1 AND materialization_id=$2`, m.TenantID, replacement.ID); err != nil {
		t.Fatal(err)
	}
	for _, commit := range []bool{false, true} {
		if err := checkWriter(t, db, w, commit, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
			t.Fatalf("old writer accepted: %v", err)
		}
		forged := w
		forged.Fence = next.FencingToken
		if err := checkWriter(t, db, forged, commit, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
			t.Fatalf("old lease with new fence accepted: %v", err)
		}
		current := w
		current.MaterializationID = replacement.ID
		current.LeaseID = next.ID
		current.Fence = next.FencingToken
		if err := checkWriter(t, db, current, commit, 1); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil || stored.State != domain.MaterializationFenced {
		t.Fatalf("old writer state: %+v %v", stored, err)
	}
}

// The guard retains the Workspace lock through the protected metadata mutation.
func TestMaterializationWriterGuardRetainsLocksPostgres(t *testing.T) {
	db, _, m, _, w := writerGuardFixture(t)
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := NewMaterializationWriterGuard(tx).ValidateCommit(t.Context(), w, 1); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := NewWorkspaceRepository(db).AdvanceWriterFence(t.Context(), m.TenantID, m.WorkspaceID)
		result <- err
	}()
	waitForWriterGuardLock(t, db, "%UPDATE thinkpixelws.workspaces%")
	// A protected lifecycle write is rolled back with the guard's transaction.
	if _, err := tx.ExecContext(t.Context(), `UPDATE thinkpixelws.materializations SET lifecycle_state='CHECKPOINTING' WHERE tenant_id=$1 AND materialization_id=$2`, m.TenantID, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	stored, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil || stored.State != domain.MaterializationActive {
		t.Fatalf("rollback: %+v %v", stored, err)
	}
	for _, commit := range []bool{false, true} {
		if err := checkWriter(t, db, w, commit, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
			t.Fatalf("stale fence accepted: %v", err)
		}
	}
}

func waitForWriterGuardLock(t *testing.T, db *sql.DB, pattern string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := db.QueryRowContext(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND ltrim(query) LIKE $1)`, pattern).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("operation did not wait for lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMaterializationWriterGuardWaitsForExpiryPostgres(t *testing.T) {
	db, _, m, l, w := writerGuardFixture(t)
	blocker, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := lockLeaseWorkspace(t.Context(), blocker, m.TenantID, m.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			result <- err
			return
		}
		defer tx.Rollback()
		result <- NewMaterializationWriterGuard(tx).ValidateCheckpoint(t.Context(), w)
	}()
	waitForWriterGuardLock(t, db, "SELECT writer_fence,head_generation%")
	ageLease(t, blocker, l)
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ports.ErrMaterializationWriterConflict) {
		t.Fatalf("expired while waiting: %v", err)
	}
}

func TestMaterializationWriterGuardRequiresSerializablePostgres(t *testing.T) {
	db, _, _, _, w := writerGuardFixture(t)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := NewMaterializationWriterGuard(tx).ValidateCommit(t.Context(), w, 1); err == nil {
		t.Fatal("nonserializable commit accepted")
	}
}

func TestMaterializationWriterGuardReleasedLeasePostgres(t *testing.T) {
	db, _, m, l, w := writerGuardFixture(t)
	if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.materialization_leases SET released_at=clock_timestamp() WHERE tenant_id=$1 AND lease_id=$2`, m.TenantID, l.ID); err != nil {
		t.Fatal(err)
	}
	for _, commit := range []bool{false, true} {
		if err := checkWriter(t, db, w, commit, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
			t.Fatalf("released lease accepted: %v", err)
		}
	}
}

func TestMaterializationWriterGuardRevalidatesPostgres(t *testing.T) {
	db, _, _, l, w := writerGuardFixture(t)
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	guard := NewMaterializationWriterGuard(tx)
	if err := guard.ValidateCheckpoint(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	ageLease(t, tx, l)
	if err := guard.ValidateCommit(t.Context(), w, 1); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
		t.Fatalf("expired after initial check: %v", err)
	}
}
