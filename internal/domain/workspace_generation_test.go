package domain

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

func TestNewWorkspaceGeneration(t *testing.T) {
	t.Parallel()

	input := validNewWorkspaceGeneration(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	generation, err := input.WorkspaceGeneration(now)
	if err != nil {
		t.Fatalf("create workspace generation: %v", err)
	}
	if generation.State != WorkspaceGenerationCompleted || generation.CreatedAt.Location() != time.UTC || !generation.CreatedAt.Equal(now) {
		t.Fatalf("unexpected workspace generation: %#v", generation)
	}
}

func TestNewWorkspaceGenerationRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewWorkspaceGeneration){
		"non-v7 tenant ID":     func(input *NewWorkspaceGeneration) { input.TenantID = uuid.New() },
		"non-v7 workspace ID":  func(input *NewWorkspaceGeneration) { input.WorkspaceID = uuid.New() },
		"non-v7 generation ID": func(input *NewWorkspaceGeneration) { input.ID = uuid.New() },
		"zero number":          func(input *NewWorkspaceGeneration) { input.Number = 0 },
		"oversized number":     func(input *NewWorkspaceGeneration) { input.Number = math.MaxInt64 + 1 },
		"non-preceding parent": func(input *NewWorkspaceGeneration) { parent := input.Number; input.ParentNumber = &parent },
		"missing digest":       func(input *NewWorkspaceGeneration) { input.ManifestDigest = shared.SHA256Digest{} },
		"invalid durability":   func(input *NewWorkspaceGeneration) { input.Durability = "local" },
		"blank creator":        func(input *NewWorkspaceGeneration) { input.CreatedByPrincipal = "" },
		"unnormalized creator": func(input *NewWorkspaceGeneration) { input.CreatedByPrincipal = " principal " },
		"non-v7 execution ID": func(input *NewWorkspaceGeneration) {
			executionID := uuid.New()
			input.CreatedByExecution = &executionID
		},
		"oversized creator": func(input *NewWorkspaceGeneration) {
			input.CreatedByPrincipal = strings.Repeat("x", maxGenerationCreatorIDLength+1)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewWorkspaceGeneration(t)
			mutate(&input)
			if _, err := input.WorkspaceGeneration(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func validNewWorkspaceGeneration(t *testing.T) NewWorkspaceGeneration {
	t.Helper()
	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	generationID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	parent := uint64(1)
	executionID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return NewWorkspaceGeneration{
		TenantID: tenantID, WorkspaceID: workspaceID, ID: generationID,
		Number: 2, ParentNumber: &parent, ManifestDigest: shared.DigestBytes([]byte("manifest")),
		Durability: GenerationDurabilityProviderLocal, CreatedByPrincipal: "principal-123",
		CreatedByExecution: &executionID,
	}
}
