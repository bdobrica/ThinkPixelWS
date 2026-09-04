package domain

import (
	"errors"
	"math"
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

func TestWorkspaceLifecycleTransitions(t *testing.T) {
	t.Parallel()

	edges := map[WorkspaceState][]WorkspaceState{
		WorkspaceCreating:  {WorkspaceReady, WorkspaceDegraded},
		WorkspaceDegraded:  {WorkspaceReady},
		WorkspaceReady:     {WorkspaceArchiving, WorkspaceDeleting},
		WorkspaceArchiving: {WorkspaceArchived, WorkspaceDegraded},
		WorkspaceArchived:  {WorkspaceRestoring, WorkspaceDeleting},
		WorkspaceRestoring: {WorkspaceReady, WorkspaceDegraded},
		WorkspaceDeleting:  {WorkspaceDeleted},
		WorkspaceDeleted:   {},
	}
	for current, nextStates := range edges {
		for _, next := range nextStates {
			if !current.CanTransitionTo(next) {
				t.Fatalf("expected transition %s to %s", current, next)
			}
		}
	}
	for current := range edges {
		for next := range edges {
			want := false
			for _, allowed := range edges[current] {
				want = want || next == allowed
			}
			if got := current.CanTransitionTo(next); got != want {
				t.Fatalf("transition %s to %s: got %t, want %t", current, next, got, want)
			}
		}
	}
}

func TestWorkspaceTransitionStateUsesExpectedVersion(t *testing.T) {
	t.Parallel()

	workspace := validDomainWorkspace(t)
	now := workspace.UpdatedAt.Add(time.Second)
	transitioned, err := workspace.TransitionState(WorkspaceReady, 1, now)
	if err != nil {
		t.Fatalf("transition state: %v", err)
	}
	if transitioned.State != WorkspaceReady || transitioned.StateVersion != 2 || !transitioned.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected transitioned workspace: %#v", transitioned)
	}
	if _, err := workspace.TransitionState(WorkspaceReady, 2, now); !errors.Is(err, ErrWorkspaceStateVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
	if _, err := workspace.TransitionState(WorkspaceArchived, 1, now); !errors.Is(err, ErrInvalidWorkspaceStateTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
	workspace.StateVersion = math.MaxInt64
	if _, err := workspace.TransitionState(WorkspaceReady, math.MaxInt64, now); err == nil {
		t.Fatal("expected exhausted state version to be rejected")
	}
}

func validDomainWorkspace(t *testing.T) Workspace {
	t.Helper()
	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := (NewWorkspace{TenantID: tenantID, ID: workspaceID, Name: "workspace", Owner: Owner{Kind: OwnerUser, ID: "owner"}}).Workspace(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return workspace
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
