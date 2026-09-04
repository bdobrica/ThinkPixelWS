package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewRepositoryComponentMetadata(t *testing.T) {
	t.Parallel()

	defaultRef := "refs/heads/main"
	input := validNewRepositoryComponentMetadata(t)
	input.DefaultRef = &defaultRef
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	metadata, err := input.RepositoryComponentMetadata(now)
	if err != nil {
		t.Fatalf("create repository component metadata: %v", err)
	}
	if metadata.DefaultRef == nil || *metadata.DefaultRef != defaultRef || metadata.CreatedAt.Location() != time.UTC || !metadata.CreatedAt.Equal(now) {
		t.Fatalf("unexpected repository component metadata: %#v", metadata)
	}
}

func TestNewRepositoryComponentMetadataAllowsNoDefaultRef(t *testing.T) {
	t.Parallel()
	if _, err := validNewRepositoryComponentMetadata(t).RepositoryComponentMetadata(time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestNewRepositoryComponentMetadataRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	invalidRef := "refs/heads/main\n"
	emptyRef := ""
	longRef := strings.Repeat("a", maxRepositoryDefaultRefLength+1)
	tests := map[string]func(*NewRepositoryComponentMetadata){
		"non-v7 tenant ID":    func(input *NewRepositoryComponentMetadata) { input.TenantID = uuid.New() },
		"non-v7 workspace ID": func(input *NewRepositoryComponentMetadata) { input.WorkspaceID = uuid.New() },
		"non-v7 component ID": func(input *NewRepositoryComponentMetadata) { input.ComponentID = uuid.New() },
		"empty vcs":           func(input *NewRepositoryComponentMetadata) { input.VersionControlSystem = "" },
		"uppercase vcs":       func(input *NewRepositoryComponentMetadata) { input.VersionControlSystem = "Git" },
		"spaced vcs":          func(input *NewRepositoryComponentMetadata) { input.VersionControlSystem = " git " },
		"empty default ref":   func(input *NewRepositoryComponentMetadata) { input.DefaultRef = &emptyRef },
		"control in ref":      func(input *NewRepositoryComponentMetadata) { input.DefaultRef = &invalidRef },
		"long default ref":    func(input *NewRepositoryComponentMetadata) { input.DefaultRef = &longRef },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewRepositoryComponentMetadata(t)
			mutate(&input)
			if _, err := input.RepositoryComponentMetadata(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func validNewRepositoryComponentMetadata(t *testing.T) NewRepositoryComponentMetadata {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewRepositoryComponentMetadata{
		TenantID: newID(), WorkspaceID: newID(), ComponentID: newID(),
		VersionControlSystem: "git",
	}
}
