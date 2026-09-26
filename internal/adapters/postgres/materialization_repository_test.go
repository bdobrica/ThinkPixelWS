package postgres

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func requestedMaterialization(t *testing.T, tenantID, workspaceID uuid.UUID) domain.Materialization {
	t.Helper()
	m, err := (domain.NewMaterialization{
		TenantID: tenantID, WorkspaceID: workspaceID, ID: uuid.Must(uuid.NewV7()), BaseGeneration: 1,
		Provider: "kubernetes", Mode: domain.MaterializationReadOnly,
		Target: domain.MaterializationTarget{ID: "test-target", Region: "eu", StorageClass: "ssd", Architecture: "arm64"},
	}).Materialization(time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMaterializationRepositoryRejectsInvalidCreate(t *testing.T) {
	m := requestedMaterialization(t, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	repository := NewMaterializationRepository(nil)
	if err := repository.Create(t.Context(), uuid.Must(uuid.NewV7()), m); err == nil {
		t.Fatal("mismatched tenant accepted")
	}
	for _, mutate := range []func(*domain.Materialization){
		func(m *domain.Materialization) { m.State = domain.MaterializationActive },
		func(m *domain.Materialization) { m.StateVersion = 2 },
		func(m *domain.Materialization) { m.UpdatedAt = m.UpdatedAt.Add(time.Second) },
		func(m *domain.Materialization) { m.BaseGeneration = 0 },
	} {
		invalid := m
		mutate(&invalid)
		if err := repository.Create(t.Context(), m.TenantID, invalid); err == nil {
			t.Fatal("invalid initial state accepted")
		}
	}
}

func TestMaterializationRepositoryPostgres(t *testing.T) {
	db, dsn := materializationTestDatabase(t)
	ctx := t.Context()
	workspace := validStoredWorkspace(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, workspace.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := NewWorkspaceRepository(db).Create(ctx, workspace.TenantID, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.workspace_generations
 (tenant_id,workspace_id,generation,generation_id,state,manifest_digest,durability,created_by_principal)
 VALUES ($1,$2,1,$3,'COMPLETED',$4,'portable','test')`, workspace.TenantID, workspace.ID, uuid.Must(uuid.NewV7()), "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	repository := NewMaterializationRepository(db)
	m := requestedMaterialization(t, workspace.TenantID, workspace.ID)
	if err := repository.Create(ctx, m.TenantID, m); err != nil {
		t.Fatal(err)
	}
	// A new connection/repository must read durable metadata, including optional architecture.
	reopened, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := NewMaterializationRepository(reopened).Get(ctx, m.TenantID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip differs: got %#v want %#v", got, m)
	}
	otherTenant := uuid.Must(uuid.NewV7())
	for _, ids := range [][2]uuid.UUID{{otherTenant, m.ID}, {m.TenantID, uuid.Must(uuid.NewV7())}} {
		if _, err := repository.Get(ctx, ids[0], ids[1]); !errors.Is(err, ports.ErrMaterializationNotFound) {
			t.Fatalf("expected not found: %v", err)
		}
	}
	if err := repository.Create(ctx, m.TenantID, m); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	// Multiple requests may coexist. No request establishes a writable lease.
	for _, mode := range []domain.MaterializationMode{domain.MaterializationReadOnly, domain.MaterializationReadWrite} {
		next := m
		next.ID = uuid.Must(uuid.NewV7())
		next.Mode = mode
		next.Target.Architecture = ""
		if err := repository.Create(ctx, next.TenantID, next); err != nil {
			t.Fatal(err)
		}
		got, err := repository.Get(ctx, next.TenantID, next.ID)
		if err != nil || !reflect.DeepEqual(got, next) {
			t.Fatalf("mode/optional field roundtrip: %#v %v", got, err)
		}
	}
	// Reference checks must fail even when bypassing domain validation.
	if _, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, otherTenant); err != nil {
		t.Fatal(err)
	}
	foreignWorkspace := workspace
	foreignWorkspace.TenantID = otherTenant
	if err := NewWorkspaceRepository(db).Create(ctx, otherTenant, foreignWorkspace); err != nil {
		t.Fatal(err)
	}
	// Even an identical Workspace UUID in another tenant cannot borrow its generation.
	foreign := m
	foreign.ID = uuid.Must(uuid.NewV7())
	foreign.TenantID = otherTenant
	assertMaterializationPGError(t, repository.Create(ctx, otherTenant, foreign), "23503")
	missing := m
	missing.ID = uuid.Must(uuid.NewV7())
	missing.BaseGeneration = 2
	assertMaterializationPGError(t, repository.Create(ctx, m.TenantID, missing), "23503")
	otherWorkspace := workspace
	otherWorkspace.ID = uuid.Must(uuid.NewV7())
	otherWorkspace.Name = "other"
	if err := NewWorkspaceRepository(db).Create(ctx, workspace.TenantID, otherWorkspace); err != nil {
		t.Fatal(err)
	}
	missing.WorkspaceID = otherWorkspace.ID
	missing.BaseGeneration = 1
	assertMaterializationPGError(t, repository.Create(ctx, m.TenantID, missing), "23503")
	for _, assignment := range []string{
		"mode='writable'", "lifecycle_state='UNKNOWN'", "state_version=0", "base_generation=0",
		"provider='invalid/provider'", "target_id=''", "target_region=''", "target_storage_class=''",
		"target_architecture=repeat('x',33)", "updated_at=created_at-interval '1 second'",
		"created_at='-infinity'", "materialization_id='00000000-0000-4000-8000-000000000000'",
	} {
		_, err := db.ExecContext(ctx, `UPDATE thinkpixelws.materializations SET `+assignment+` WHERE tenant_id=$1 AND materialization_id=$2`, m.TenantID, m.ID)
		assertMaterializationPGError(t, err, "23514")
	}
	// Shared transactions allow the business layer to atomically add audit/outbox.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	uncommitted := m
	uncommitted.ID = uuid.Must(uuid.NewV7())
	if err := NewMaterializationRepository(tx).Create(ctx, m.TenantID, uncommitted); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, m.TenantID, uncommitted.ID); !errors.Is(err, ports.ErrMaterializationNotFound) {
		t.Fatalf("rollback retained row: %v", err)
	}
	// The new migration can be rolled back and reapplied without changing canonical state.
	for _, suffix := range []string{"down", "up"} {
		data, err := os.ReadFile("../../../migrations/000023_materializations." + suffix + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM thinkpixelws.workspace_generations WHERE tenant_id=$1 AND workspace_id=$2`, m.TenantID, m.WorkspaceID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("canonical generation changed: %d %v", count, err)
	}
	if err := repository.Create(ctx, m.TenantID, m); err != nil {
		t.Fatal(err)
	}
}

func assertMaterializationPGError(t *testing.T, err error, code string) {
	t.Helper()
	var pg *pq.Error
	if !errors.As(err, &pg) || string(pg.Code) != code {
		t.Fatalf("expected PostgreSQL %s, got %v", code, err)
	}
}

func materializationTestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("THINKPIXELWS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THINKPIXELWS_TEST_DATABASE_URL for isolated PostgreSQL integration")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := "mat001_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	paths, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(paths) == 0 {
		t.Fatal("migrations not found")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(data)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return db, u.String()
}
