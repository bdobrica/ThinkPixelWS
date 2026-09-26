package postgres

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func TestMultipleReadOnlyMaterializationsPostgres(t *testing.T) {
	db, dsn := materializationTestDatabase(t)
	ctx := t.Context()
	w := validStoredWorkspace(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, w.TenantID); err != nil {
		t.Fatal(err)
	}
	workspaces := NewWorkspaceRepository(db)
	if err := workspaces.Create(ctx, w.TenantID, w); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.workspace_generations
 (tenant_id,workspace_id,generation,generation_id,state,manifest_digest,durability,created_by_principal)
 VALUES ($1,$2,1,$3,'COMPLETED',$4,'portable','test')`, w.TenantID, w.ID, uuid.Must(uuid.NewV7()), "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	mats := NewMaterializationRepository(db)
	leases := NewMaterializationLeaseRepository(db)
	advance := func(m *domain.Materialization, next domain.MaterializationState) error {
		if err := mats.TransitionState(ctx, m.TenantID, m.ID, m.State, next, m.StateVersion, m.UpdatedAt); err != nil {
			return err
		}
		m.State, m.StateVersion = next, m.StateVersion+1
		return nil
	}
	var readers []domain.Materialization
	// Readers coexist on the same generation and target, both before and after
	// a writer reserves the Workspace. No reader needs a writable lease.
	for _, withWriter := range []bool{false, true} {
		if withWriter {
			writer := requestedMaterialization(t, w.TenantID, w.ID)
			writer.Mode = domain.MaterializationReadWrite
			if err := mats.Create(ctx, w.TenantID, writer); err != nil {
				t.Fatal(err)
			}
			lease, err := leases.Acquire(ctx, w.TenantID, writer.ID, uuid.Must(uuid.NewV7()), "writer")
			if err != nil || lease.FencingToken != 1 {
				t.Fatalf("readers blocked writer: %+v %v", lease, err)
			}
		}
		const batch = 4
		results := make(chan error, batch)
		start := make(chan struct{})
		pending := make([]domain.Materialization, batch)
		for i := range pending {
			pending[i] = requestedMaterialization(t, w.TenantID, w.ID)
			go func() {
				<-start
				m := &pending[i]
				err := mats.Create(ctx, w.TenantID, *m)
				if err == nil {
					err = advance(m, domain.MaterializationPreparing)
				}
				if err == nil {
					m.Handle = domain.MaterializationHandle("reader-" + m.ID.String())
					err = mats.Bind(ctx, m.TenantID, m.ID, m.Handle, m.StateVersion, m.UpdatedAt)
					m.StateVersion++
				}
				for _, next := range []domain.MaterializationState{domain.MaterializationReady, domain.MaterializationActive} {
					if err == nil {
						err = advance(m, next)
					}
				}
				results <- err
			}()
		}
		close(start)
		for range batch {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		readers = append(readers, pending...)
		wantFence := uint64(0)
		if withWriter {
			wantFence = 1
		}
		stored, err := workspaces.Get(ctx, w.TenantID, w.ID)
		if err != nil || stored.WriterFence != wantFence {
			t.Fatalf("readers changed writer fence: %+v %v", stored, err)
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND workspace_id=$2`, w.TenantID, w.ID).Scan(&count); err != nil || count != int(wantFence) {
			t.Fatalf("readers consumed leases: %d %v", count, err)
		}
	}
	// A reader's release is independent of every other reader and the writer.
	for _, next := range []domain.MaterializationState{domain.MaterializationReleasing, domain.MaterializationReleased} {
		if err := advance(&readers[0], next); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, want := range readers {
		got, err := NewMaterializationRepository(reopened).Get(ctx, w.TenantID, want.ID)
		if err != nil || got != want {
			t.Fatalf("durable independent reader: got %+v want %+v: %v", got, want, err)
		}
		if _, err := mats.Get(ctx, uuid.Must(uuid.NewV7()), want.ID); !errors.Is(err, ports.ErrMaterializationNotFound) {
			t.Fatalf("reader tenant isolation: %v", err)
		}
	}
	stored, err := NewWorkspaceRepository(reopened).Get(ctx, w.TenantID, w.ID)
	if err != nil || stored.WriterFence != 1 {
		t.Fatalf("reader release changed fence: %+v %v", stored, err)
	}
	var currentWriters int
	if err := reopened.QueryRowContext(ctx, `SELECT count(*) FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND workspace_id=$2 AND released_at IS NULL AND fencing_token=1`, w.TenantID, w.ID).Scan(&currentWriters); err != nil || currentWriters != 1 {
		t.Fatalf("reader release changed writer slot: %d %v", currentWriters, err)
	}
}
