package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

func TestNewComponentProvenance(t *testing.T) {
	t.Parallel()
	input := validNewComponentProvenance(t)
	now := time.Date(2026, time.September, 4, 20, 0, 0, 0, time.FixedZone("test", 3*60*60))
	provenance, err := input.ComponentProvenance(now)
	if err != nil {
		t.Fatalf("create component provenance: %v", err)
	}
	if provenance.ImportedAt.Location() != time.UTC || !provenance.ImportedAt.Equal(now) ||
		len(provenance.DerivedFrom) != 1 || len(provenance.Taints) != 2 {
		t.Fatalf("unexpected component provenance: %#v", provenance)
	}
	input.Taints[0] = "changed"
	if provenance.Taints[0] == "changed" {
		t.Fatal("component provenance retained mutable input slice")
	}
}

func TestNewComponentProvenanceAcceptsContractVocabularies(t *testing.T) {
	t.Parallel()
	for _, classification := range []ProvenanceClassification{ProvenancePublic, ProvenanceInternal, ProvenanceConfidential, ProvenanceRestricted} {
		for _, trust := range []ProvenanceSourceTrust{ProvenanceTrustedInternal, ProvenanceAuthenticatedExternal, ProvenanceExternalUntrusted} {
			input := validNewComponentProvenance(t)
			input.Classification, input.SourceTrust = classification, trust
			if _, err := input.ComponentProvenance(time.Now()); err != nil {
				t.Fatalf("classification %q, trust %q: %v", classification, trust, err)
			}
		}
	}
}

func TestNewComponentProvenanceRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*NewComponentProvenance){
		"non-v7 tenant ID":  func(input *NewComponentProvenance) { input.TenantID = uuid.New() },
		"zero generation":   func(input *NewComponentProvenance) { input.Generation = 0 },
		"relative source":   func(input *NewComponentProvenance) { input.Source = "repository/project" },
		"credential source": func(input *NewComponentProvenance) { input.Source = "https://user:secret@example.test/project" },
		"empty source type": func(input *NewComponentProvenance) { input.SourceType = "" },
		"long source revision": func(input *NewComponentProvenance) {
			input.SourceRevision = strings.Repeat("x", maxSourceRevisionLength+1)
		},
		"blank principal":        func(input *NewComponentProvenance) { input.InitiatingPrincipal = " principal " },
		"non-v7 Run ID":          func(input *NewComponentProvenance) { id := uuid.New(); input.SourceRunID = &id },
		"zero derivation digest": func(input *NewComponentProvenance) { input.DerivedFrom = []shared.SHA256Digest{{}} },
		"unknown classification": func(input *NewComponentProvenance) { input.Classification = "secret" },
		"unknown trust":          func(input *NewComponentProvenance) { input.SourceTrust = "verified" },
		"empty taint":            func(input *NewComponentProvenance) { input.Taints = []string{""} },
		"control taint":          func(input *NewComponentProvenance) { input.Taints = []string{"risk\nlabel"} },
		"duplicate taint":        func(input *NewComponentProvenance) { input.Taints = []string{"high-taint", "high-taint"} },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewComponentProvenance(t)
			mutate(&input)
			if _, err := input.ComponentProvenance(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewComponentProvenanceRejectsZeroImportTime(t *testing.T) {
	t.Parallel()
	if _, err := validNewComponentProvenance(t).ComponentProvenance(time.Time{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewComponentProvenance(t *testing.T) NewComponentProvenance {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	runID := newID()
	return NewComponentProvenance{
		TenantID: newID(), WorkspaceID: newID(), Generation: 2, ComponentID: newID(),
		Source: "https://example.test/engineering/project", SourceType: "git",
		SourceRevision: "0123456789abcdef", InitiatingPrincipal: "principal-123",
		SourceRunID: &runID, DerivedFrom: []shared.SHA256Digest{shared.DigestBytes([]byte("parent"))},
		Classification: ProvenanceInternal, SourceTrust: ProvenanceAuthenticatedExternal,
		Taints: []string{"source:external", "high-taint"},
	}
}
