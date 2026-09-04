package domain

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewWorkspaceFork(t *testing.T) {
	t.Parallel()

	input := validNewWorkspaceFork(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	fork, err := input.WorkspaceFork(now)
	if err != nil {
		t.Fatalf("create Workspace fork: %v", err)
	}
	if fork.ChildGeneration != 1 || fork.CreatedAt.Location() != time.UTC || !fork.CreatedAt.Equal(now) {
		t.Fatalf("unexpected Workspace fork: %#v", fork)
	}
}

func TestNewWorkspaceForkRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewWorkspaceFork){
		"non-v7 tenant ID":     func(input *NewWorkspaceFork) { input.TenantID = uuid.New() },
		"non-v7 source ID":     func(input *NewWorkspaceFork) { input.SourceWorkspaceID = uuid.New() },
		"non-v7 child ID":      func(input *NewWorkspaceFork) { input.ChildWorkspaceID = uuid.New() },
		"same Workspace":       func(input *NewWorkspaceFork) { input.ChildWorkspaceID = input.SourceWorkspaceID },
		"zero generation":      func(input *NewWorkspaceFork) { input.SourceGeneration = 0 },
		"oversized generation": func(input *NewWorkspaceFork) { input.SourceGeneration = math.MaxInt64 + 1 },
		"blank creator":        func(input *NewWorkspaceFork) { input.CreatedByPrincipal = "" },
		"unnormalized creator": func(input *NewWorkspaceFork) { input.CreatedByPrincipal = " principal " },
		"control creator":      func(input *NewWorkspaceFork) { input.CreatedByPrincipal = "principal\n" },
		"oversized creator": func(input *NewWorkspaceFork) {
			input.CreatedByPrincipal = strings.Repeat("x", maxGenerationCreatorIDLength+1)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewWorkspaceFork(t)
			mutate(&input)
			if _, err := input.WorkspaceFork(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWorkspaceForkRejectsNonInitialChildGeneration(t *testing.T) {
	t.Parallel()

	fork, err := validNewWorkspaceFork(t).WorkspaceFork(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	fork.ChildGeneration = 2
	if err := fork.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewWorkspaceFork(t *testing.T) NewWorkspaceFork {
	t.Helper()
	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	sourceWorkspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	childWorkspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return NewWorkspaceFork{
		TenantID: tenantID, SourceWorkspaceID: sourceWorkspaceID,
		SourceGeneration: 42, ChildWorkspaceID: childWorkspaceID,
		CreatedByPrincipal: "principal-123",
	}
}
