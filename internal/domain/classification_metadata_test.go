package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewComponentClassificationMetadata(t *testing.T) {
	t.Parallel()
	input := validNewComponentClassificationMetadata(t)
	metadata, err := input.ComponentClassificationMetadata()
	if err != nil {
		t.Fatalf("create component classification metadata: %v", err)
	}
	input.Taints[0] = "changed"
	if metadata.Taints[0] == "changed" {
		t.Fatal("component classification metadata retained mutable input slice")
	}
}

func TestComponentClassificationMetadataAcceptsContractClassifications(t *testing.T) {
	t.Parallel()
	for _, classification := range []Classification{
		ClassificationPublic, ClassificationInternal,
		ClassificationConfidential, ClassificationRestricted,
	} {
		input := validNewComponentClassificationMetadata(t)
		input.Classification = classification
		if _, err := input.ComponentClassificationMetadata(); err != nil {
			t.Fatalf("classification %q: %v", classification, err)
		}
	}
}

func TestComponentClassificationMetadataRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*NewComponentClassificationMetadata){
		"non-v7 tenant ID": func(input *NewComponentClassificationMetadata) { input.TenantID = uuid.New() },
		"zero generation":  func(input *NewComponentClassificationMetadata) { input.Generation = 0 },
		"unknown classification": func(input *NewComponentClassificationMetadata) {
			input.Classification = "secret"
		},
		"empty taint": func(input *NewComponentClassificationMetadata) { input.Taints = []string{""} },
		"long taint": func(input *NewComponentClassificationMetadata) {
			input.Taints = []string{strings.Repeat("x", maxTaintLength+1)}
		},
		"control taint": func(input *NewComponentClassificationMetadata) { input.Taints = []string{"risk\nlabel"} },
		"duplicate taint": func(input *NewComponentClassificationMetadata) {
			input.Taints = []string{"high-taint", "high-taint"}
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewComponentClassificationMetadata(t)
			mutate(&input)
			if _, err := input.ComponentClassificationMetadata(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func validNewComponentClassificationMetadata(t *testing.T) NewComponentClassificationMetadata {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewComponentClassificationMetadata{
		TenantID: newID(), WorkspaceID: newID(), Generation: 3,
		ComponentID: newID(), Classification: ClassificationConfidential,
		Taints: []string{"source:external", "high-taint"},
	}
}
