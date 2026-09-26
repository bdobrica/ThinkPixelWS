package postgres

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

func TestMaterializationLeaseSchemaPostgres(t *testing.T) {
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
	repo := NewMaterializationRepository(db)
	if err := repo.Create(ctx, m.TenantID, m); err != nil {
		t.Fatal(err)
	}
	lease, err := (domain.NewMaterializationLease{ID: uuid.Must(uuid.NewV7()), FencingToken: 1, Holder: "execution-ref"}).MaterializationLease(m, m.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO thinkpixelws.materialization_leases
 (tenant_id,lease_id,workspace_id,materialization_id,fencing_token,holder,issued_at,renewed_at,expires_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	args := []any{lease.TenantID, lease.ID, lease.WorkspaceID, lease.MaterializationID, lease.FencingToken, lease.Holder, lease.IssuedAt, lease.RenewedAt, lease.ExpiresAt}
	if _, err := db.ExecContext(ctx, insert, args...); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var got domain.MaterializationLease
	if err := reopened.QueryRowContext(ctx, `SELECT tenant_id,lease_id,workspace_id,materialization_id,fencing_token,holder,issued_at,renewed_at,expires_at,released_at
 FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND lease_id=$2`, lease.TenantID, lease.ID).Scan(
		&got.TenantID, &got.ID, &got.WorkspaceID, &got.MaterializationID, &got.FencingToken, &got.Holder, &got.IssuedAt, &got.RenewedAt, &got.ExpiresAt, &got.ReleasedAt); err != nil {
		t.Fatal(err)
	}
	got.IssuedAt, got.RenewedAt, got.ExpiresAt = got.IssuedAt.UTC(), got.RenewedAt.UTC(), got.ExpiresAt.UTC()
	if got != lease {
		t.Fatalf("durable lease differs: %#v want %#v", got, lease)
	}
	if _, err := db.ExecContext(ctx, insert, args...); err == nil {
		t.Fatal("duplicate lease accepted")
	}
	ro := m
	ro.ID = uuid.Must(uuid.NewV7())
	ro.Mode = domain.MaterializationReadOnly
	if err := repo.Create(ctx, ro.TenantID, ro); err != nil {
		t.Fatal(err)
	}
	for _, assignment := range []string{
		"tenant_id='" + uuid.Must(uuid.NewV7()).String() + "'",
		"workspace_id='" + uuid.Must(uuid.NewV7()).String() + "'",
		"materialization_id='" + uuid.Must(uuid.NewV7()).String() + "'",
		"materialization_id='" + ro.ID.String() + "'",
	} {
		_, err := db.ExecContext(ctx, `UPDATE thinkpixelws.materialization_leases SET `+assignment+` WHERE tenant_id=$1 AND lease_id=$2`, lease.TenantID, lease.ID)
		assertMaterializationPGError(t, err, "23503")
	}
	for _, assignment := range []string{
		"materialization_mode='read-only'", "fencing_token=0", "holder=''", "holder=' padded'", "holder=E'control\\n'", "holder=repeat('x',257)",
		"lease_id='00000000-0000-4000-8000-000000000000'", "issued_at='-infinity'", "expires_at='infinity'", "renewed_at=issued_at-interval '1 second'", "expires_at=renewed_at", "released_at=renewed_at-interval '1 second'",
	} {
		_, err := db.ExecContext(ctx, `UPDATE thinkpixelws.materialization_leases SET `+assignment+` WHERE tenant_id=$1 AND lease_id=$2`, lease.TenantID, lease.ID)
		assertMaterializationPGError(t, err, "23514")
	}
	// A parent cannot become read-only while leases reference its writable mode.
	_, err = db.ExecContext(ctx, `UPDATE thinkpixelws.materializations SET mode='read-only' WHERE tenant_id=$1 AND materialization_id=$2`, m.TenantID, m.ID)
	assertMaterializationPGError(t, err, "23503")
	released := lease.ExpiresAt.Add(time.Second)
	if _, err := db.ExecContext(ctx, `UPDATE thinkpixelws.materialization_leases SET released_at=$3 WHERE tenant_id=$1 AND lease_id=$2`, lease.TenantID, lease.ID, released); err != nil {
		t.Fatal(err)
	}
	// Rollback removes lease metadata only, retaining Materializations and generations.
	for _, migration := range []string{"000026_current_writable_lease.down", "000025_materialization_leases.down", "000025_materialization_leases.up", "000026_current_writable_lease.up"} {
		data, err := os.ReadFile("../../../migrations/" + migration + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	persisted, err := repo.Get(ctx, m.TenantID, m.ID)
	if err != nil || persisted != m {
		t.Fatalf("canonical metadata changed: %#v %v", persisted, err)
	}
	if _, err := db.ExecContext(ctx, insert, args...); err != nil {
		t.Fatal(err)
	}
}
