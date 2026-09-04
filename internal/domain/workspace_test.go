package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewWorkspace(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	workspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate workspace ID: %v", err)
	}
	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate tenant ID: %v", err)
	}

	workspace, err := (NewWorkspace{
		TenantID:  tenantID,
		ID:        workspaceID,
		Name:      "product-docs",
		Owner:     Owner{Kind: OwnerTeam, ID: "documentation"},
		Residency: []string{"jurisdiction:eu", "region:ro"},
	}).Workspace(now)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if workspace.State != WorkspaceCreating || workspace.StateVersion != 1 {
		t.Fatalf("unexpected initial state: state=%q version=%d", workspace.State, workspace.StateVersion)
	}
	if workspace.Classification != ClassificationInternal {
		t.Fatalf("unexpected default classification: %q", workspace.Classification)
	}
	if workspace.CreatedAt.Location() != time.UTC || !workspace.CreatedAt.Equal(now) {
		t.Fatalf("creation time was not normalized to UTC: %v", workspace.CreatedAt)
	}
}

func TestNewWorkspaceRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	valid := NewWorkspace{
		TenantID: tenantID,
		ID:       workspaceID,
		Name:     "workspace",
		Owner:    Owner{Kind: OwnerUser, ID: "owner"},
	}

	tests := map[string]func(*NewWorkspace){
		"non-v7 tenant ID":    func(input *NewWorkspace) { input.TenantID = uuid.New() },
		"non-v7 workspace ID": func(input *NewWorkspace) { input.ID = uuid.New() },
		"invalid name":        func(input *NewWorkspace) { input.Name = "Not Normalized" },
		"invalid owner":       func(input *NewWorkspace) { input.Owner.ID = " owner " },
		"invalid owner kind":  func(input *NewWorkspace) { input.Owner.Kind = "group" },
		"long description": func(input *NewWorkspace) {
			input.Description = strings.Repeat("x", maxWorkspaceDescriptionLength+1)
		},
		"duplicate residency": func(input *NewWorkspace) {
			input.Residency = []string{"region:ro", "region:ro"}
		},
		"invalid residency": func(input *NewWorkspace) { input.Residency = []string{" region:ro"} },
	}

	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := valid
			mutate(&input)
			if _, err := input.Workspace(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
