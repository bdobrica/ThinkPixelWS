package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewWorkspaceComponent(t *testing.T) {
	t.Parallel()

	input := validNewWorkspaceComponent(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	component, err := input.WorkspaceComponent(now)
	if err != nil {
		t.Fatalf("create workspace component: %v", err)
	}
	if component.CanonicalPath != "/workspace/source" || component.CreatedAt.Location() != time.UTC || !component.CreatedAt.Equal(now) {
		t.Fatalf("unexpected workspace component: %#v", component)
	}
}

func TestNewWorkspaceComponentAcceptsAllKinds(t *testing.T) {
	t.Parallel()

	kinds := []WorkspaceComponentKind{
		WorkspaceComponentRepository, WorkspaceComponentDirectory,
		WorkspaceComponentDocumentCollection, WorkspaceComponentArtifactCollection,
	}
	for _, kind := range kinds {
		input := validNewWorkspaceComponent(t)
		input.Kind = kind
		if _, err := input.WorkspaceComponent(time.Now()); err != nil {
			t.Errorf("kind %q: %v", kind, err)
		}
	}
}

func TestNewWorkspaceComponentRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewWorkspaceComponent){
		"non-v7 tenant ID":    func(input *NewWorkspaceComponent) { input.TenantID = uuid.New() },
		"non-v7 workspace ID": func(input *NewWorkspaceComponent) { input.WorkspaceID = uuid.New() },
		"non-v7 component ID": func(input *NewWorkspaceComponent) { input.ID = uuid.New() },
		"empty name":          func(input *NewWorkspaceComponent) { input.Name = "" },
		"uppercase name":      func(input *NewWorkspaceComponent) { input.Name = "Source" },
		"nested name":         func(input *NewWorkspaceComponent) { input.Name = "src/code" },
		"invalid kind":        func(input *NewWorkspaceComponent) { input.Kind = "external" },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewWorkspaceComponent(t)
			mutate(&input)
			if _, err := input.WorkspaceComponent(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWorkspaceComponentRejectsNonCanonicalPath(t *testing.T) {
	t.Parallel()

	component, err := validNewWorkspaceComponent(t).WorkspaceComponent(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	component.CanonicalPath = "/workspace/other"
	if err := component.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewWorkspaceComponent(t *testing.T) NewWorkspaceComponent {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewWorkspaceComponent{
		TenantID: newID(), WorkspaceID: newID(), ID: newID(),
		Name: "source", Kind: WorkspaceComponentRepository,
	}
}
