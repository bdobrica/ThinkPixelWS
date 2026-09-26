package postgres

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func TestMaterializationLeaseAcquisitionPostgres(t *testing.T) {
	db, dsn := materializationTestDatabase(t)
	ctx := t.Context()
	w := validStoredWorkspace(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, w.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := NewWorkspaceRepository(db).Create(ctx, w.TenantID, w); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.workspace_generations
 (tenant_id,workspace_id,generation,generation_id,state,manifest_digest,durability,created_by_principal)
 VALUES ($1,$2,1,$3,'COMPLETED',$4,'portable','test')`, w.TenantID, w.ID, uuid.Must(uuid.NewV7()), "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	m := requestedMaterialization(t, w.TenantID, w.ID)
	m.Mode = domain.MaterializationReadWrite
	mats := NewMaterializationRepository(db)
	if err := mats.Create(ctx, m.TenantID, m); err != nil {
		t.Fatal(err)
	}
	second := m
	second.ID = uuid.Must(uuid.NewV7())
	if err := mats.Create(ctx, second.TenantID, second); err != nil {
		t.Fatal(err)
	}
	repo := NewMaterializationLeaseRepository(db)
	acquire := func(tenant, mat uuid.UUID, holder string) (domain.MaterializationLease, error) {
		return repo.Acquire(ctx, tenant, mat, uuid.Must(uuid.NewV7()), holder)
	}
	assertFence := func(want uint64) {
		t.Helper()
		stored, err := NewWorkspaceRepository(db).Get(ctx, w.TenantID, w.ID)
		if err != nil || stored.WriterFence != want {
			t.Fatalf("fence=%d want %d: %v", stored.WriterFence, want, err)
		}
	}
	if _, err := acquire(uuid.Must(uuid.NewV7()), m.ID, "execution"); !errors.Is(err, ports.ErrMaterializationNotFound) {
		t.Fatalf("tenant scope: %v", err)
	}
	if _, err := acquire(w.TenantID, uuid.Must(uuid.NewV7()), "execution"); !errors.Is(err, ports.ErrMaterializationNotFound) {
		t.Fatalf("missing materialization: %v", err)
	}
	if _, err := acquire(w.TenantID, m.ID, ""); err == nil {
		t.Fatal("invalid holder accepted")
	}
	assertFence(0)
	ro := m
	ro.ID = uuid.Must(uuid.NewV7())
	ro.Mode = domain.MaterializationReadOnly
	if err := mats.Create(ctx, ro.TenantID, ro); err != nil {
		t.Fatal(err)
	}
	if _, err := acquire(w.TenantID, ro.ID, "execution"); !errors.Is(err, ports.ErrMaterializationLeaseIneligible) {
		t.Fatalf("read-only acquired: %v", err)
	}
	assertFence(0)
	type result struct {
		lease domain.MaterializationLease
		err   error
	}
	const writers = 12
	results := make(chan result, writers)
	start := make(chan struct{})
	for i := range writers {
		id := m.ID
		if i%2 == 1 {
			id = second.ID
		}
		go func() { <-start; l, e := acquire(w.TenantID, id, "execution"); results <- result{l, e} }()
	}
	close(start)
	successes := 0
	var winner domain.MaterializationLease
	for range writers {
		r := <-results
		if r.err == nil {
			successes++
			winner = r.lease
		} else if !errors.Is(r.err, ports.ErrWritableLeaseConflict) {
			t.Fatal(r.err)
		}
	}
	if successes != 1 || winner.FencingToken != 1 {
		t.Fatalf("winners=%d lease=%+v", successes, winner)
	}
	if winner.ExpiresAt.Sub(winner.IssuedAt) != 60*time.Second {
		t.Fatal("unexpected TTL")
	}
	assertFence(1)
	reopened, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.QueryRowContext(ctx, `SELECT count(*) FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND workspace_id=$2 AND released_at IS NULL AND fencing_token=1`, w.TenantID, w.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable winner count=%d: %v", count, err)
	}
	// Direct SQL cannot bypass the writer slot; released history can coexist.
	insert := `INSERT INTO thinkpixelws.materialization_leases
 (tenant_id,lease_id,workspace_id,materialization_id,fencing_token,holder,issued_at,renewed_at,expires_at)
 SELECT tenant_id,$3,workspace_id,materialization_id,fencing_token+1,holder,issued_at,renewed_at,expires_at
 FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND lease_id=$2`
	_, err = db.ExecContext(ctx, insert, w.TenantID, winner.ID, uuid.Must(uuid.NewV7()))
	assertMaterializationPGError(t, err, "23505")
	if _, err := db.ExecContext(ctx, `UPDATE thinkpixelws.materialization_leases SET released_at=clock_timestamp() WHERE tenant_id=$1 AND lease_id=$2`, w.TenantID, winner.ID); err != nil {
		t.Fatal(err)
	}
	// A primary-key failure after allocation must roll back the fence as well.
	if _, err := repo.Acquire(ctx, w.TenantID, m.ID, winner.ID, "execution"); err == nil {
		t.Fatal("duplicate lease accepted")
	}
	assertFence(1)
	next, err := acquire(w.TenantID, second.ID, "execution-2")
	if err != nil || next.FencingToken != 2 {
		t.Fatalf("replacement=%+v: %v", next, err)
	}
	assertFence(2)
	// Other Workspaces and tenants have independent writer slots and fences.
	for _, foreignTenant := range []bool{false, true} {
		other := w
		other.ID = uuid.Must(uuid.NewV7())
		if foreignTenant {
			other.ID = w.ID // Same Workspace ID, different tenant scope.
			other.TenantID = uuid.Must(uuid.NewV7())
			if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, other.TenantID); err != nil {
				t.Fatal(err)
			}
		}
		if err := NewWorkspaceRepository(db).Create(ctx, other.TenantID, other); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.workspace_generations
 (tenant_id,workspace_id,generation,generation_id,state,manifest_digest,durability,created_by_principal)
 VALUES ($1,$2,1,$3,'COMPLETED',$4,'portable','test')`, other.TenantID, other.ID, uuid.Must(uuid.NewV7()), "sha256:"+strings.Repeat("a", 64)); err != nil {
			t.Fatal(err)
		}
		candidate := requestedMaterialization(t, other.TenantID, other.ID)
		candidate.Mode = domain.MaterializationReadWrite
		if err := mats.Create(ctx, candidate.TenantID, candidate); err != nil {
			t.Fatal(err)
		}
		l, err := acquire(candidate.TenantID, candidate.ID, "independent")
		if err != nil || l.FencingToken != 1 {
			t.Fatalf("independent slot: %+v %v", l, err)
		}
	}
	terminal := m
	terminal.ID = uuid.Must(uuid.NewV7())
	if err := mats.Create(ctx, terminal.TenantID, terminal); err != nil {
		t.Fatal(err)
	}
	if err := mats.TransitionState(ctx, terminal.TenantID, terminal.ID, domain.MaterializationRequested, domain.MaterializationFailed, 1, terminal.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := acquire(terminal.TenantID, terminal.ID, "terminal"); !errors.Is(err, ports.ErrMaterializationLeaseIneligible) {
		t.Fatalf("terminal acquired: %v", err)
	}
	assertFence(2)
	// Rolling back the index retains data; reapplying restores enforcement.
	for _, direction := range []string{"down", "up"} {
		data, err := os.ReadFile("../../../migrations/000026_current_writable_lease." + direction + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.ExecContext(ctx, insert, w.TenantID, next.ID, uuid.Must(uuid.NewV7()))
	assertMaterializationPGError(t, err, "23505")
}
