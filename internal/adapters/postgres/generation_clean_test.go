package postgres

import (
	"errors"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func TestGenerationCleanPostgres(t *testing.T) {
	for _, finish := range []string{"resume", "failed", "expiry", "unconfirmed commit", "confirmed commit"} {
		t.Run(finish, func(t *testing.T) {
			db, leases, m, lease, w := writerGuardFixture(t)
			repo := NewMaterializationRepository(db)
			in := generationCommitInput(w)
			in.MarkClean = true
			if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
				t.Fatalf("ACTIVE clean claim: %v", err)
			}
			assertCommitCounts(t, db, 1, 1, 0)
			if err := repo.TransitionState(t.Context(), m.TenantID, m.ID, domain.MaterializationActive, domain.MaterializationCheckpointing, 1, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			in.MaterializationVersion = 2
			g, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			clean, err := repo.Get(t.Context(), m.TenantID, m.ID)
			if err != nil || clean.CleanGeneration != g.Number || clean.StateVersion != 3 || clean.BaseGeneration != 1 || clean.State != domain.MaterializationCheckpointing || clean.Handle != m.Handle {
				t.Fatalf("clean readback: %+v %v", clean, err)
			}
			if err := repo.TransitionState(t.Context(), m.TenantID, m.ID, clean.State, domain.MaterializationActive, 2, time.Now().UTC()); !errors.Is(err, ports.ErrMaterializationStateConflict) {
				t.Fatalf("stale transition: %v", err)
			}
			switch finish {
			case "resume", "failed":
				next := domain.MaterializationActive
				if finish == "failed" {
					next = domain.MaterializationFailed
				}
				if err := repo.TransitionState(t.Context(), m.TenantID, m.ID, clean.State, next, clean.StateVersion, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
			case "expiry":
				ageLease(t, db, lease)
				if expired, err := leases.Expire(t.Context(), m.TenantID, m.WorkspaceID); err != nil || !expired {
					t.Fatalf("expiry: %v %v", expired, err)
				}
			default:
				in.GenerationID = uuid.Must(uuid.NewV7())
				in.ExpectedHead = 2
				if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
					t.Fatalf("stale commit: %v", err)
				}
				in.MaterializationVersion = clean.StateVersion
				in.MarkClean = finish == "confirmed commit"
				if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); err != nil {
					t.Fatal(err)
				}
				assertCommitCounts(t, db, 3, 3, 2)
			}
			after, err := repo.Get(t.Context(), m.TenantID, m.ID)
			var want uint64
			if finish == "confirmed commit" {
				want = 3
			}
			if err != nil || after.CleanGeneration != want || after.StateVersion != 4 || after.BaseGeneration != 1 {
				t.Fatalf("after %s: %+v %v", finish, after, err)
			}
		})
	}
}

func TestGenerationCleanUnknownAndExhaustedPostgres(t *testing.T) {
	db, _, m, _, w := writerGuardFixture(t)
	if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.materializations SET lifecycle_state='CHECKPOINTING',state_version=$1`, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	in := generationCommitInput(w)
	in.MaterializationVersion = math.MaxInt64
	in.MarkClean = true
	if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); !errors.Is(err, ports.ErrMaterializationWriterConflict) {
		t.Fatalf("exhausted version: %v", err)
	}
	assertCommitCounts(t, db, 1, 1, 0)
	in.MarkClean = false
	if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	got, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil || got.CleanGeneration != 0 || got.StateVersion != math.MaxInt64 {
		t.Fatalf("unconfirmed capture: %+v %v", got, err)
	}
}

func TestGenerationCleanMigrationPostgres(t *testing.T) {
	db, _, m, _, _ := writerGuardFixture(t)
	before, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000029_materialization_clean_generation.down.sql", "000029_materialization_clean_generation.up.sql"} {
		migration, err := os.ReadFile("../../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	after, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("historical record: %+v %v", after, err)
	}
	for _, update := range []string{
		"clean_generation=1", // ACTIVE cannot be clean.
		"lifecycle_state='CHECKPOINTING',clean_generation=0",
		"lifecycle_state='CHECKPOINTING',mode='read-only',clean_generation=1",
		"lifecycle_state='CHECKPOINTING',clean_generation=2", // No such generation in this Workspace.
	} {
		if _, err := db.ExecContext(t.Context(), "UPDATE thinkpixelws.materializations SET "+update); err == nil {
			t.Fatalf("invalid clean marker accepted: %s", update)
		}
	}
}
