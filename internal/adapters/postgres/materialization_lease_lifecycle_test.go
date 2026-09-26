package postgres

import (
	"database/sql"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func leaseLifecycleFixture(t *testing.T) (*sql.DB, *MaterializationLeaseRepository, domain.Materialization, domain.MaterializationLease) {
	t.Helper()
	db, _ := materializationTestDatabase(t)
	ctx := t.Context()
	w := validStoredWorkspace(t)
	if _, err := db.ExecContext(ctx, "INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')", w.TenantID); err != nil {
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
	if err := NewMaterializationRepository(db).Create(ctx, m.TenantID, m); err != nil {
		t.Fatal(err)
	}
	repo := NewMaterializationLeaseRepository(db)
	lease, err := repo.Acquire(ctx, m.TenantID, m.ID, uuid.Must(uuid.NewV7()), "execution")
	if err != nil {
		t.Fatal(err)
	}
	return db, repo, m, lease
}

func ageLease(t *testing.T, db database, l domain.MaterializationLease) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.materialization_leases
 SET issued_at=issued_at-interval '2 minutes',renewed_at=renewed_at-interval '2 minutes',
 expires_at=expires_at-interval '2 minutes' WHERE tenant_id=$1 AND lease_id=$2`, l.TenantID, l.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializationLeaseRenewalPostgres(t *testing.T) {
	db, repo, m, lease := leaseLifecycleFixture(t)
	ctx := t.Context()
	renew := func(l domain.MaterializationLease) (domain.MaterializationLease, error) {
		return repo.Renew(ctx, l.TenantID, l.MaterializationID, l.ID, l.FencingToken, l.Holder)
	}
	for _, field := range []string{"tenant", "materialization", "lease", "holder", "fence", "overflow"} {
		bad := lease
		switch field {
		case "tenant":
			bad.TenantID = uuid.Must(uuid.NewV7())
		case "materialization":
			bad.MaterializationID = uuid.Must(uuid.NewV7())
		case "lease":
			bad.ID = uuid.Must(uuid.NewV7())
		case "holder":
			bad.Holder = "other"
		case "fence":
			bad.FencingToken++
		case "overflow":
			bad.FencingToken = math.MaxUint64
		}
		if got, err := renew(bad); !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) || got != (domain.MaterializationLease{}) {
			t.Fatalf("%s: %+v %v", field, got, err)
		}
	}
	for _, state := range []domain.MaterializationState{domain.MaterializationRequested, domain.MaterializationPreparing, domain.MaterializationReady, domain.MaterializationActive, domain.MaterializationCheckpointing} {
		if _, err := db.ExecContext(ctx, "UPDATE thinkpixelws.materializations SET lifecycle_state=$3 WHERE tenant_id=$1 AND materialization_id=$2", m.TenantID, m.ID, state); err != nil {
			t.Fatal(err)
		}
		got, err := renew(lease)
		if err != nil {
			t.Fatal(err)
		}
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
		if got.ID != lease.ID || got.FencingToken != lease.FencingToken || !got.IssuedAt.Equal(lease.IssuedAt) ||
			!got.RenewedAt.After(lease.RenewedAt) || got.ExpiresAt.Sub(got.RenewedAt) != 60*time.Second {
			t.Fatalf("renewed metadata: %+v", got)
		}
		lease = got
	}
	var renewed, expires time.Time
	if err := db.QueryRowContext(ctx, "SELECT renewed_at,expires_at FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND lease_id=$2", lease.TenantID, lease.ID).Scan(&renewed, &expires); err != nil || !renewed.Equal(lease.RenewedAt) || !expires.Equal(lease.ExpiresAt) {
		t.Fatalf("persisted renewal: %v", err)
	}
	if expired, err := repo.Expire(ctx, m.TenantID, m.WorkspaceID); err != nil || expired {
		t.Fatalf("unexpired lease retired: %v %v", expired, err)
	}
	for _, state := range []domain.MaterializationState{domain.MaterializationReleasing, domain.MaterializationReleased, domain.MaterializationFailed, domain.MaterializationFenced} {
		if _, err := db.ExecContext(ctx, "UPDATE thinkpixelws.materializations SET lifecycle_state=$3 WHERE tenant_id=$1 AND materialization_id=$2", m.TenantID, m.ID, state); err != nil {
			t.Fatal(err)
		}
		if _, err := renew(lease); !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) {
			t.Fatalf("%s renewed: %v", state, err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE thinkpixelws.materializations SET lifecycle_state='ACTIVE' WHERE tenant_id=$1 AND materialization_id=$2", m.TenantID, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceRepository(db).AdvanceWriterFence(ctx, m.TenantID, m.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := renew(lease); !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) {
		t.Fatalf("stale fence renewed: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE thinkpixelws.workspaces SET writer_fence=1 WHERE tenant_id=$1 AND workspace_id=$2", m.TenantID, m.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE thinkpixelws.materialization_leases SET released_at=clock_timestamp() WHERE tenant_id=$1 AND lease_id=$2", lease.TenantID, lease.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := renew(lease); !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) {
		t.Fatalf("released lease renewed: %v", err)
	}
}

func TestMaterializationLeaseExpiryPostgres(t *testing.T) {
	db, repo, m, lease := leaseLifecycleFixture(t)
	ctx := t.Context()
	ageLease(t, db, lease)
	if _, err := repo.Renew(ctx, lease.TenantID, m.ID, lease.ID, lease.FencingToken, lease.Holder); !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) {
		t.Fatalf("expired renewal: %v", err)
	}
	if _, err := repo.Expire(ctx, uuid.Must(uuid.NewV7()), m.WorkspaceID); !errors.Is(err, ports.ErrWorkspaceNotFound) {
		t.Fatalf("foreign expiry: %v", err)
	}
	replacement := m
	replacement.ID = uuid.Must(uuid.NewV7())
	if err := NewMaterializationRepository(db).Create(ctx, replacement.TenantID, replacement); err != nil {
		t.Fatal(err)
	}
	// Retirement and fencing must roll back with a failed replacement insertion.
	if _, err := repo.Acquire(ctx, m.TenantID, replacement.ID, lease.ID, "replacement"); err == nil {
		t.Fatal("duplicate lease accepted")
	}
	stored, err := NewMaterializationRepository(db).Get(ctx, m.TenantID, m.ID)
	if err != nil || stored.State != m.State || stored.StateVersion != m.StateVersion {
		t.Fatalf("retirement not rolled back: %+v %v", stored, err)
	}
	var released sql.NullTime
	if err := db.QueryRowContext(ctx, "SELECT released_at FROM thinkpixelws.materialization_leases WHERE tenant_id=$1 AND lease_id=$2", lease.TenantID, lease.ID).Scan(&released); err != nil || released.Valid {
		t.Fatalf("release not rolled back: %v %v", released, err)
	}
	type result struct {
		l   domain.MaterializationLease
		err error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	for range 8 {
		go func() {
			<-start
			l, err := repo.Acquire(ctx, m.TenantID, replacement.ID, uuid.Must(uuid.NewV7()), "replacement")
			results <- result{l, err}
		}()
	}
	close(start)
	var winner domain.MaterializationLease
	successes := 0
	for range 8 {
		r := <-results
		if r.err == nil {
			successes++
			winner = r.l
		} else if !errors.Is(r.err, ports.ErrWritableLeaseConflict) {
			t.Fatal(r.err)
		}
	}
	if successes != 1 || winner.FencingToken != 2 {
		t.Fatalf("replacement winners=%d lease=%+v", successes, winner)
	}
	stored, err = NewMaterializationRepository(db).Get(ctx, m.TenantID, m.ID)
	if err != nil || stored.State != domain.MaterializationFenced || stored.StateVersion != m.StateVersion+1 {
		t.Fatalf("stale writer not fenced: %+v %v", stored, err)
	}
	if _, err := repo.Renew(ctx, lease.TenantID, m.ID, lease.ID, lease.FencingToken, lease.Holder); !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) {
		t.Fatalf("old writer renewed after replacement: %v", err)
	}
	if _, err := repo.Acquire(ctx, m.TenantID, m.ID, uuid.Must(uuid.NewV7()), "old"); !errors.Is(err, ports.ErrMaterializationLeaseIneligible) {
		t.Fatalf("old writer reacquired: %v", err)
	}
	ageLease(t, db, winner)
	for i := range 2 {
		expired, err := repo.Expire(ctx, m.TenantID, m.WorkspaceID)
		if err != nil || expired != (i == 0) {
			t.Fatalf("expiry %d: %v %v", i, expired, err)
		}
	}
	w, err := NewWorkspaceRepository(db).Get(ctx, m.TenantID, m.WorkspaceID)
	if err != nil || w.WriterFence != 2 {
		t.Fatalf("expiry changed fence: %+v %v", w, err)
	}
}

func TestMaterializationLeaseRenewalWaitsForWorkspacePostgres(t *testing.T) {
	db, repo, m, lease := leaseLifecycleFixture(t)
	ctx := t.Context()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := lockLeaseWorkspace(ctx, tx, m.TenantID, m.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := repo.Renew(ctx, lease.TenantID, m.ID, lease.ID, lease.FencingToken, lease.Holder)
		result <- err
	}()
	// Wait until renewal is blocked on the Workspace, then expire the lease
	// behind that lock. A pre-lock time/eligibility snapshot must not renew it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM pg_stat_activity WHERE datname=current_database()
 AND wait_event_type='Lock' AND query LIKE 'SELECT writer_fence%')`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("renewal did not wait for Workspace lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ageLease(t, tx, lease)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ports.ErrMaterializationLeaseNotRenewable) {
		t.Fatalf("expired while waiting: %v", err)
	}
}

func TestMaterializationLeaseExpiryStatesPostgres(t *testing.T) {
	for _, state := range []domain.MaterializationState{domain.MaterializationActive, domain.MaterializationCheckpointing, domain.MaterializationFailed} {
		t.Run(string(state), func(t *testing.T) {
			db, repo, m, lease := leaseLifecycleFixture(t)
			ctx := t.Context()
			if _, err := db.ExecContext(ctx, "UPDATE thinkpixelws.materializations SET lifecycle_state=$3 WHERE tenant_id=$1 AND materialization_id=$2", m.TenantID, m.ID, state); err != nil {
				t.Fatal(err)
			}
			ageLease(t, db, lease)
			if retired, err := repo.Expire(ctx, m.TenantID, m.WorkspaceID); err != nil || !retired {
				t.Fatalf("expiry: %v %v", retired, err)
			}
			wantState, wantVersion := domain.MaterializationFenced, m.StateVersion+1
			if state == domain.MaterializationFailed {
				wantState, wantVersion = state, m.StateVersion
			}
			stored, err := NewMaterializationRepository(db).Get(ctx, m.TenantID, m.ID)
			if err != nil || stored.State != wantState || stored.StateVersion != wantVersion {
				t.Fatalf("expired state: %+v %v", stored, err)
			}
		})
	}
}

func TestMaterializationLeaseRenewalCompetitionPostgres(t *testing.T) {
	db, repo, m, lease := leaseLifecycleFixture(t)
	ctx := t.Context()
	start := make(chan struct{})
	results := make(chan error, 3)
	go func() {
		<-start
		_, err := repo.Renew(ctx, m.TenantID, m.ID, lease.ID, lease.FencingToken, lease.Holder)
		results <- err
	}()
	go func() {
		<-start
		expired, err := repo.Expire(ctx, m.TenantID, m.WorkspaceID)
		if expired {
			err = errors.New("live writer expired")
		}
		results <- err
	}()
	go func() {
		<-start
		_, err := repo.Acquire(ctx, m.TenantID, m.ID, uuid.Must(uuid.NewV7()), "contender")
		if errors.Is(err, ports.ErrWritableLeaseConflict) {
			err = nil
		} else if err == nil {
			err = errors.New("live writer replaced")
		}
		results <- err
	}()
	close(start)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	w, err := NewWorkspaceRepository(db).Get(ctx, m.TenantID, m.WorkspaceID)
	if err != nil || w.WriterFence != 1 {
		t.Fatalf("renewal competition changed fence: %+v %v", w, err)
	}
}
