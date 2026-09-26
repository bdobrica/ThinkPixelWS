package postgres

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func generationCommitInput(w ports.MaterializationWriter) ports.GenerationCommit {
	return ports.GenerationCommit{
		Writer: w, ExpectedHead: 1, MaterializationVersion: 1,
		GenerationID: uuid.Must(uuid.NewV7()), ManifestDigest: shared.DigestBytes([]byte("prepared immutable manifest")),
		Durability: domain.GenerationDurabilityPortable, Principal: "commit-test",
	}
}

func TestGenerationCommitInvalidInput(t *testing.T) {
	w := ports.MaterializationWriter{TenantID: uuid.Must(uuid.NewV7()), WorkspaceID: uuid.Must(uuid.NewV7())}
	for _, mutate := range []func(*ports.GenerationCommit){
		func(in *ports.GenerationCommit) { in.ExpectedHead = 0 },
		func(in *ports.GenerationCommit) { in.ExpectedHead = math.MaxInt64 },
		func(in *ports.GenerationCommit) { in.MaterializationVersion = 0 },
		func(in *ports.GenerationCommit) { in.ManifestDigest = shared.SHA256Digest{} },
		func(in *ports.GenerationCommit) { in.Durability = "unknown" },
		func(in *ports.GenerationCommit) { id := uuid.Nil; in.RunID = &id },
		func(in *ports.GenerationCommit) { id := uuid.New(); in.RunID = &id },
		func(in *ports.GenerationCommit) { in.Principal = "" },
		func(in *ports.GenerationCommit) { in.GenerationID = uuid.Nil },
	} {
		in := generationCommitInput(w)
		mutate(&in)
		if _, err := (GenerationCommitter{}).Commit(t.Context(), in); err == nil || err.Error() == "generation persistence is not configured" {
			t.Fatalf("invalid input reached persistence: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (GenerationCommitter{}).Commit(ctx, generationCommitInput(w)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func assertCommitCounts(t *testing.T, db *sql.DB, head, generations, records int) {
	t.Helper()
	var gotHead, gotGenerations, audits, messages int
	if err := db.QueryRowContext(t.Context(), `SELECT head_generation,
 (SELECT count(*) FROM thinkpixelws.workspace_generations),
 (SELECT count(*) FROM thinkpixelws.audit_events),
 (SELECT count(*) FROM thinkpixelws.outbox_messages) FROM thinkpixelws.workspaces`).Scan(&gotHead, &gotGenerations, &audits, &messages); err != nil {
		t.Fatal(err)
	}
	if gotHead != head || gotGenerations != generations || audits != records || messages != records {
		t.Fatalf("head/generations/audit/outbox = %d/%d/%d/%d; want %d/%d/%d/%d", gotHead, gotGenerations, audits, messages, head, generations, records, records)
	}
}

func TestGenerationCommitPostgres(t *testing.T) {
	db, _, m, _, w := writerGuardFixture(t)
	in := generationCommitInput(w)
	execution := uuid.Must(uuid.NewV7())
	in.ExecutionID = &execution
	run := uuid.Must(uuid.NewV7())
	in.RunID = &run
	before, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	g, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := (WorkspaceReader{DB: db}).GetGeneration(t.Context(), m.TenantID, m.WorkspaceID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, stored) || g.ID != in.GenerationID || g.Number != 2 || g.ParentNumber == nil || *g.ParentNumber != 1 || g.ManifestDigest != in.ManifestDigest || g.CreatedByRun == nil || *g.CreatedByRun != run || g.CreatedByPrincipal != in.Principal || g.CreatedByExecution == nil || *g.CreatedByExecution != execution {
		t.Fatalf("generation did not round trip: %+v %+v", g, stored)
	}
	assertCommitCounts(t, db, 2, 2, 1)
	var linked bool
	if err := db.QueryRowContext(t.Context(), `SELECT EXISTS (
 SELECT 1 FROM thinkpixelws.audit_events a JOIN thinkpixelws.outbox_messages o USING (tenant_id,transaction_id)
 WHERE a.target_id=$1 AND o.aggregate_id=$2 AND a.metadata=o.payload
 AND a.action='workspace.commit' AND o.event_type='workspace.thinkpixel.io/generation.committed.v1')`, g.ID.String(), g.ID).Scan(&linked); err != nil || !linked {
		t.Fatalf("audit/outbox not linked: %v", err)
	}
	after, err := NewMaterializationRepository(db).Get(t.Context(), m.TenantID, m.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("materialization changed: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.workspace_generations SET manifest_digest=$1 WHERE generation=2`, shared.DigestBytes([]byte("replacement")).String()); err == nil {
		t.Fatal("completed generation was mutable")
	}
	// Repeating an old expected head never creates a second generation.
	if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); !errors.Is(err, ports.ErrWorkspaceHeadConflict) {
		t.Fatalf("replay: %v", err)
	}
	assertCommitCounts(t, db, 2, 2, 1)
	// The same current writer can publish another capture without rebasing its
	// materialization or overwriting its original immutable base.
	in.RunID, in.ExecutionID = nil, nil
	in.ExpectedHead = 2
	in.GenerationID = uuid.Must(uuid.NewV7())
	if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	stored, err = (WorkspaceReader{DB: db}).GetGeneration(t.Context(), m.TenantID, m.WorkspaceID, 3)
	if err != nil || stored.CreatedByRun != nil || stored.CreatedByExecution != nil || stored.CreatedByPrincipal != in.Principal {
		t.Fatalf("optional provenance readback: %+v %v", stored, err)
	}
	assertCommitCounts(t, db, 3, 3, 2)
}

func TestGenerationCommitRejectsWriterPostgres(t *testing.T) {
	db, _, m, lease, w := writerGuardFixture(t)
	for _, field := range []string{"tenant", "materialization", "lease", "fence", "version", "head", "read-only", "fenced", "expired"} {
		t.Run(field, func(t *testing.T) {
			in := generationCommitInput(w)
			want := ports.ErrMaterializationWriterConflict
			switch field {
			case "tenant":
				in.Writer.TenantID = uuid.Must(uuid.NewV7())
			case "materialization":
				in.Writer.MaterializationID = uuid.Must(uuid.NewV7())
			case "lease":
				in.Writer.LeaseID = uuid.Must(uuid.NewV7())
			case "fence":
				in.Writer.Fence++
			case "version":
				in.MaterializationVersion++
			case "head":
				in.ExpectedHead++
				want = ports.ErrWorkspaceHeadConflict
			case "read-only":
				ro := requestedMaterialization(t, m.TenantID, m.WorkspaceID)
				if err := NewMaterializationRepository(db).Create(t.Context(), ro.TenantID, ro); err != nil {
					t.Fatal(err)
				}
				in.Writer.MaterializationID = ro.ID
			case "fenced":
				if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.materializations SET lifecycle_state='FENCED' WHERE materialization_id=$1`, m.ID); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if _, err := db.Exec(`UPDATE thinkpixelws.materializations SET lifecycle_state='ACTIVE' WHERE materialization_id=$1`, m.ID); err != nil {
						t.Error(err)
					}
				})
			case "expired":
				ageLease(t, db, lease)
			}
			if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in); !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			assertCommitCounts(t, db, 1, 1, 0)
		})
	}
}

func TestGenerationCommitRollbackPostgres(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "outbox failure", true: "expiry during publication"}[expire], func(t *testing.T) {
			db, _, _, _, w := writerGuardFixture(t)
			body := `RAISE EXCEPTION 'injected outbox failure';`
			if expire {
				body = `UPDATE thinkpixelws.materialization_leases SET issued_at=issued_at-interval '2 minutes', renewed_at=renewed_at-interval '2 minutes', expires_at=expires_at-interval '2 minutes'; RETURN NEW;`
			}
			if _, err := db.ExecContext(t.Context(), `CREATE FUNCTION thinkpixelws.test_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+body+` END $$;
 CREATE TRIGGER test_commit_failure BEFORE INSERT ON thinkpixelws.outbox_messages FOR EACH ROW EXECUTE FUNCTION thinkpixelws.test_commit_failure()`); err != nil {
				t.Fatal(err)
			}
			in := generationCommitInput(w)
			in.ComponentReferences = commitComponentReferences(t, db, w)
			_, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in)
			if err == nil || (expire && !errors.Is(err, ports.ErrMaterializationWriterConflict)) {
				t.Fatalf("failure not propagated: %v", err)
			}
			assertCommitCounts(t, db, 1, 1, 0)
		})
	}
}

func TestGenerationCommitConcurrentPostgres(t *testing.T) {
	db, _, _, _, w := writerGuardFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := (GenerationCommitter{DB: db}).Commit(t.Context(), generationCommitInput(w))
			results <- err
		}()
	}
	close(start)
	success := 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
			continue
		}
		var pg *pq.Error
		if !errors.Is(err, ports.ErrWorkspaceHeadConflict) && !(errors.As(err, &pg) && pg.Code == "40001") {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("%d commits succeeded", success)
	}
	assertCommitCounts(t, db, 2, 2, 1)
}

func TestGenerationRunProvenanceMigrationPostgres(t *testing.T) {
	db, _, m, _, _ := writerGuardFixture(t)
	// Applying the migration to preexisting immutable generations must preserve
	// their absence of Run attribution. Rollback/reapplication also remains valid.
	for _, name := range []string{"000027_generation_run_provenance.down.sql", "000027_generation_run_provenance.up.sql"} {
		migration, err := os.ReadFile("../../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	g, err := (WorkspaceReader{DB: db}).GetGeneration(t.Context(), m.TenantID, m.WorkspaceID, 1)
	if err != nil || g.CreatedByRun != nil {
		t.Fatalf("existing generation: %+v %v", g, err)
	}
	for _, id := range []uuid.UUID{uuid.Nil, uuid.New()} {
		_, err := db.ExecContext(t.Context(), `INSERT INTO thinkpixelws.workspace_generations
   (tenant_id,workspace_id,generation,generation_id,parent_generation,state,manifest_digest,durability,created_by_principal,created_by_run_id)
   SELECT tenant_id,workspace_id,2,$1,1,state,manifest_digest,durability,created_by_principal,$2
   FROM thinkpixelws.workspace_generations WHERE generation=1`, uuid.Must(uuid.NewV7()), id)
		var pgErr *pq.Error
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.Constraint != "workspace_generations_run_id_uuidv7" {
			t.Fatalf("invalid Run ID: %v", err)
		}
	}
}
