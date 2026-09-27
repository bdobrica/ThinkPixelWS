package postgres

import (
	"os"
	"reflect"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

func TestMaterializationExecutionReferencesPostgres(t *testing.T) {
	db, _, base, _ := leaseLifecycleFixture(t)
	repo := NewMaterializationRepository(db)
	ctx := t.Context()
	for _, refs := range [][2]uuid.UUID{{}, {uuid.Must(uuid.NewV7()), uuid.Nil}, {uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}} {
		m := requestedMaterialization(t, base.TenantID, base.WorkspaceID)
		m.RunID, m.ExecutionID = refs[0], refs[1]
		if err := repo.Create(ctx, m.TenantID, m); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Get(ctx, m.TenantID, m.ID)
		if err != nil || !reflect.DeepEqual(got, m) {
			t.Fatalf("roundtrip: %#v %v", got, err)
		}
		if err := repo.TransitionState(ctx, m.TenantID, m.ID, m.State, domain.MaterializationPreparing, 1, m.UpdatedAt); err != nil {
			t.Fatal(err)
		}
		got, err = repo.Get(ctx, m.TenantID, m.ID)
		if err != nil || got.RunID != m.RunID || got.ExecutionID != m.ExecutionID {
			t.Fatalf("transition lost references: %#v %v", got, err)
		}
		for _, column := range []string{"run_id", "execution_id"} {
			_, err := db.ExecContext(ctx, `UPDATE thinkpixelws.materializations SET `+column+`=$3 WHERE tenant_id=$1 AND materialization_id=$2`, m.TenantID, m.ID, uuid.Must(uuid.NewV7()))
			assertMaterializationPGError(t, err, "23514")
		}
	}
	// Bypass Go validation to exercise database constraints on insert.
	for _, refs := range [][2]uuid.UUID{{uuid.Nil, uuid.Must(uuid.NewV7())}, {uuid.New(), uuid.Nil}, {uuid.Must(uuid.NewV7()), uuid.New()}} {
		_, err := db.ExecContext(ctx, `INSERT INTO thinkpixelws.materializations
  (tenant_id,materialization_id,workspace_id,base_generation,provider,target_id,target_region,target_storage_class,mode,lifecycle_state,run_id,execution_id)
  VALUES ($1,$2,$3,1,'kubernetes','local','local','local-path','read-only','REQUESTED',NULLIF($4::uuid,'00000000-0000-0000-0000-000000000000'::uuid),NULLIF($5::uuid,'00000000-0000-0000-0000-000000000000'::uuid))`, base.TenantID, uuid.Must(uuid.NewV7()), base.WorkspaceID, refs[0], refs[1])
		assertMaterializationPGError(t, err, "23514")
	}
	for _, direction := range []string{"down", "up"} {
		migration, err := os.ReadFile("../../../migrations/000030_materialization_execution_references." + direction + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Get(ctx, base.TenantID, base.ID)
	if err != nil || got.RunID != uuid.Nil || got.ExecutionID != uuid.Nil {
		t.Fatalf("legacy migration: %#v %v", got, err)
	}
}
