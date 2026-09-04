package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewExternalBinding(t *testing.T) {
	t.Parallel()
	classification := ExternalBindingConfidential
	input := validNewExternalBinding(t)
	input.Classification = &classification
	now := time.Date(2026, time.September, 4, 18, 0, 0, 0, time.FixedZone("test", 3*60*60))
	binding, err := input.ExternalBinding(now)
	if err != nil {
		t.Fatalf("create external binding: %v", err)
	}
	if binding.Name != input.Name || binding.Kind != input.Kind || binding.Ref != input.Ref ||
		binding.Mode != input.Mode || binding.Classification == nil ||
		*binding.Classification != classification || binding.CreatedAt.Location() != time.UTC ||
		!binding.CreatedAt.Equal(now) {
		t.Fatalf("unexpected external binding: %#v", binding)
	}
}

func TestNewExternalBindingAllowsNoClassification(t *testing.T) {
	t.Parallel()
	if _, err := validNewExternalBinding(t).ExternalBinding(time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestNewExternalBindingAcceptsContractVocabulary(t *testing.T) {
	t.Parallel()
	kinds := []ExternalBindingKind{
		ExternalBindingCollaboration, ExternalBindingIssueTracker,
		ExternalBindingDocumentStore, ExternalBindingDatabase,
		ExternalBindingService, ExternalBindingObservability,
	}
	for _, kind := range kinds {
		input := validNewExternalBinding(t)
		input.Kind = kind
		if _, err := input.ExternalBinding(time.Now()); err != nil {
			t.Fatalf("kind %q: %v", kind, err)
		}
	}
	for _, mode := range []ExternalBindingMode{ExternalBindingLiveReference, ExternalBindingMaterializedCopy} {
		input := validNewExternalBinding(t)
		input.Mode = mode
		if _, err := input.ExternalBinding(time.Now()); err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}
	for _, classification := range []ExternalBindingClassification{
		ExternalBindingPublic, ExternalBindingInternal,
		ExternalBindingConfidential, ExternalBindingRestricted,
	} {
		input := validNewExternalBinding(t)
		input.Classification = &classification
		if _, err := input.ExternalBinding(time.Now()); err != nil {
			t.Fatalf("classification %q: %v", classification, err)
		}
	}
}

func TestNewExternalBindingRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	invalidClassification := ExternalBindingClassification("secret")
	tests := map[string]func(*NewExternalBinding){
		"non-v7 tenant ID":       func(input *NewExternalBinding) { input.TenantID = uuid.New() },
		"non-v7 workspace ID":    func(input *NewExternalBinding) { input.WorkspaceID = uuid.New() },
		"invalid name":           func(input *NewExternalBinding) { input.Name = "Team Chat" },
		"long name":              func(input *NewExternalBinding) { input.Name = "a" + strings.Repeat("b", 63) },
		"unknown kind":           func(input *NewExternalBinding) { input.Kind = "queue" },
		"relative ref":           func(input *NewExternalBinding) { input.Ref = "slack/acme/payments" },
		"credential-bearing ref": func(input *NewExternalBinding) { input.Ref = "https://user:secret@example.test/channel" },
		"control in ref":         func(input *NewExternalBinding) { input.Ref = "collaboration://slack/channel\n" },
		"long ref":               func(input *NewExternalBinding) { input.Ref = "urn:" + strings.Repeat("a", maxSourceRefLength) },
		"unknown mode":           func(input *NewExternalBinding) { input.Mode = "profile-reference" },
		"unknown classification": func(input *NewExternalBinding) { input.Classification = &invalidClassification },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewExternalBinding(t)
			mutate(&input)
			if _, err := input.ExternalBinding(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewExternalBindingRejectsZeroCreationTime(t *testing.T) {
	t.Parallel()
	if _, err := validNewExternalBinding(t).ExternalBinding(time.Time{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewExternalBinding(t *testing.T) NewExternalBinding {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewExternalBinding{
		TenantID: newID(), WorkspaceID: newID(), Name: "team-chat",
		Kind: ExternalBindingCollaboration, Ref: "collaboration://slack/acme/payments",
		Mode: ExternalBindingLiveReference,
	}
}
