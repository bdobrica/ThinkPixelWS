package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewSourceBinding(t *testing.T) {
	t.Parallel()

	revision := "9f4c2d1"
	input := validNewSourceBinding(t)
	input.LastResolvedRevision = &revision
	now := time.Date(2026, time.September, 4, 15, 0, 0, 0, time.FixedZone("test", 3*60*60))
	binding, err := input.SourceBinding(now)
	if err != nil {
		t.Fatalf("create source binding: %v", err)
	}
	if binding.Provider != input.Provider || binding.Ref != input.Ref || binding.Mode != input.Mode ||
		binding.LastResolvedRevision == nil || *binding.LastResolvedRevision != revision ||
		binding.CreatedAt.Location() != time.UTC || !binding.CreatedAt.Equal(now) {
		t.Fatalf("unexpected source binding: %#v", binding)
	}
}

func TestNewSourceBindingAllowsNoResolvedRevision(t *testing.T) {
	t.Parallel()
	if _, err := validNewSourceBinding(t).SourceBinding(time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestNewSourceBindingRejectsZeroCreationTime(t *testing.T) {
	t.Parallel()
	if _, err := validNewSourceBinding(t).SourceBinding(time.Time{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNewSourceBindingAcceptsContractModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []SourceBindingMode{
		SourceBindingSnapshot,
		SourceBindingRefreshableSnapshot,
		SourceBindingLiveReference,
	} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			input := validNewSourceBinding(t)
			input.Mode = mode
			if _, err := input.SourceBinding(time.Now()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNewSourceBindingRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	emptyRevision := ""
	controlRevision := "main\n"
	longRevision := strings.Repeat("a", maxSourceRevisionLength+1)
	tests := map[string]func(*NewSourceBinding){
		"non-v7 tenant ID":       func(input *NewSourceBinding) { input.TenantID = uuid.New() },
		"non-v7 workspace ID":    func(input *NewSourceBinding) { input.WorkspaceID = uuid.New() },
		"non-v7 component ID":    func(input *NewSourceBinding) { input.ComponentID = uuid.New() },
		"empty provider":         func(input *NewSourceBinding) { input.Provider = "" },
		"uppercase provider":     func(input *NewSourceBinding) { input.Provider = "Git" },
		"long provider":          func(input *NewSourceBinding) { input.Provider = strings.Repeat("a", maxSourceProviderLength+1) },
		"relative ref":           func(input *NewSourceBinding) { input.Ref = "owner/repository" },
		"credential-bearing ref": func(input *NewSourceBinding) { input.Ref = "https://user:secret@example.test/repository" },
		"control in ref":         func(input *NewSourceBinding) { input.Ref = "https://example.test/repository\n" },
		"long ref":               func(input *NewSourceBinding) { input.Ref = "urn:" + strings.Repeat("a", maxSourceRefLength) },
		"unknown mode":           func(input *NewSourceBinding) { input.Mode = "continuous" },
		"empty revision":         func(input *NewSourceBinding) { input.LastResolvedRevision = &emptyRevision },
		"control in revision":    func(input *NewSourceBinding) { input.LastResolvedRevision = &controlRevision },
		"long revision":          func(input *NewSourceBinding) { input.LastResolvedRevision = &longRevision },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewSourceBinding(t)
			mutate(&input)
			if _, err := input.SourceBinding(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func validNewSourceBinding(t *testing.T) NewSourceBinding {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewSourceBinding{
		TenantID: newID(), WorkspaceID: newID(), ComponentID: newID(),
		Provider: "git", Ref: "https://example.test/owner/repository.git",
		Mode: SourceBindingRefreshableSnapshot,
	}
}
