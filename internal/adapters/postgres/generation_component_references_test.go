package postgres

import (
	"database/sql"
	"os"
	"reflect"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func commitComponentReferences(t *testing.T, db *sql.DB, w ports.MaterializationWriter) []domain.GenerationComponentReference {
	t.Helper()
	refs := make([]domain.GenerationComponentReference, 0, 2)
	for _, name := range []string{"code", "notes"} {
		id := uuid.Must(uuid.NewV7())
		if _, err := db.ExecContext(t.Context(), `INSERT INTO thinkpixelws.workspace_components
  (tenant_id,workspace_id,component_id,name,kind,canonical_path) VALUES ($1,$2,$3,$4,'directory',$5)`, w.TenantID, w.WorkspaceID, id, name, "/workspace/"+name); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, domain.GenerationComponentReference{ComponentID: id, Kind: domain.GenerationReferencePortableSnapshot, Ref: shared.DigestBytes([]byte("snapshot manifest for " + name)).String()})
	}
	return refs
}

func TestGenerationComponentReferencesPostgres(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "portable", true: "provider-local"}[local], func(t *testing.T) {
			db, _, _, _, w := writerGuardFixture(t)
			in := generationCommitInput(w)
			in.ComponentReferences = commitComponentReferences(t, db, w)
			if local {
				in.Durability = domain.GenerationDurabilityProviderLocal
				in.ComponentReferences[1] = domain.GenerationComponentReference{ComponentID: in.ComponentReferences[1].ComponentID, Kind: domain.GenerationReferenceProviderCheckpoint, Ref: "immutable-snapshot-uid", Provider: "kubernetes", TargetID: "homelab"}
			}
			for _, bad := range [][]domain.GenerationComponentReference{
				nil, in.ComponentReferences[:1],
				{in.ComponentReferences[0], {ComponentID: uuid.Must(uuid.NewV7()), Kind: domain.GenerationReferencePortableSnapshot, Ref: in.ComponentReferences[0].Ref}},
			} {
				candidate := in
				candidate.ComponentReferences = bad
				if _, err := (GenerationCommitter{DB: db}).Commit(t.Context(), candidate); err == nil {
					t.Fatal("incomplete or foreign references accepted")
				}
				assertCommitCounts(t, db, 1, 1, 0)
			}
			g, err := (GenerationCommitter{DB: db}).Commit(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			reader := WorkspaceReader{DB: db}
			got, err := reader.GetGeneration(t.Context(), w.TenantID, w.WorkspaceID, 2)
			if err != nil || !reflect.DeepEqual(g.ComponentReferences, in.ComponentReferences) || !reflect.DeepEqual(g, got) {
				t.Fatalf("reference readback: %+v %v", got, err)
			}
			listed, err := reader.ListGenerations(t.Context(), w.TenantID, w.WorkspaceID, 1, 10)
			if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0], g) {
				t.Fatalf("reference listing: %+v %v", listed, err)
			}
			if _, err := reader.GetGeneration(t.Context(), uuid.Must(uuid.NewV7()), w.WorkspaceID, 2); err == nil {
				t.Fatal("cross-tenant read accepted")
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE thinkpixelws.workspace_generations SET component_references='[]' WHERE generation=2`); err == nil {
				t.Fatal("references were mutable")
			}
			assertCommitCounts(t, db, 2, 2, 1)
		})
	}
}

func TestGenerationComponentReferencesMigrationPostgres(t *testing.T) {
	db, _, m, _, _ := writerGuardFixture(t)
	for _, name := range []string{"000028_generation_component_references.down.sql", "000028_generation_component_references.up.sql"} {
		migration, err := os.ReadFile("../../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	g, err := (WorkspaceReader{DB: db}).GetGeneration(t.Context(), m.TenantID, m.WorkspaceID, 1)
	if err != nil || g.ComponentReferences != nil {
		t.Fatalf("historical references invented: %+v %v", g, err)
	}
}
