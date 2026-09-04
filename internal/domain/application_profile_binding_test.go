package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewApplicationProfileBinding(t *testing.T) {
	t.Parallel()
	policy := "authenticated-browser-30d"
	input := validNewApplicationProfileBinding(t)
	input.RetentionPolicy = &policy
	now := time.Date(2026, time.September, 4, 18, 0, 0, 0, time.FixedZone("test", 3*60*60))
	binding, err := input.ApplicationProfileBinding(now)
	if err != nil {
		t.Fatalf("create application profile binding: %v", err)
	}
	if binding.Name != input.Name || binding.Kind != input.Kind || binding.Ref != input.Ref ||
		binding.RetentionPolicy == nil || *binding.RetentionPolicy != policy ||
		binding.CreatedAt.Location() != time.UTC || !binding.CreatedAt.Equal(now) {
		t.Fatalf("unexpected application profile binding: %#v", binding)
	}
}

func TestNewApplicationProfileBindingAllowsNoRetentionPolicy(t *testing.T) {
	t.Parallel()
	if _, err := validNewApplicationProfileBinding(t).ApplicationProfileBinding(time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestNewApplicationProfileBindingAcceptsContractKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []ApplicationProfileBindingKind{
		ApplicationProfileBindingBrowser, ApplicationProfileBindingDesktop,
		ApplicationProfileBindingApplication, ApplicationProfileBindingIDE,
	} {
		input := validNewApplicationProfileBinding(t)
		input.Kind = kind
		if _, err := input.ApplicationProfileBinding(time.Now()); err != nil {
			t.Fatalf("kind %q: %v", kind, err)
		}
	}
}

func TestNewApplicationProfileBindingRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*NewApplicationProfileBinding){
		"non-v7 tenant ID":       func(input *NewApplicationProfileBinding) { input.TenantID = uuid.New() },
		"non-v7 workspace ID":    func(input *NewApplicationProfileBinding) { input.WorkspaceID = uuid.New() },
		"invalid name":           func(input *NewApplicationProfileBinding) { input.Name = "Corporate Browser" },
		"long name":              func(input *NewApplicationProfileBinding) { input.Name = "a" + strings.Repeat("b", 63) },
		"unknown kind":           func(input *NewApplicationProfileBinding) { input.Kind = "mobile" },
		"relative ref":           func(input *NewApplicationProfileBinding) { input.Ref = "browser/alice/payments" },
		"credential-bearing ref": func(input *NewApplicationProfileBinding) { input.Ref = "https://user:secret@example.test/profile" },
		"control in ref":         func(input *NewApplicationProfileBinding) { input.Ref = "profile://browser/alice\n" },
		"long ref": func(input *NewApplicationProfileBinding) {
			input.Ref = "urn:" + strings.Repeat("a", maxSourceRefLength)
		},
		"empty retention policy": func(input *NewApplicationProfileBinding) { value := ""; input.RetentionPolicy = &value },
		"long retention policy": func(input *NewApplicationProfileBinding) {
			value := strings.Repeat("a", 129)
			input.RetentionPolicy = &value
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewApplicationProfileBinding(t)
			mutate(&input)
			if _, err := input.ApplicationProfileBinding(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewApplicationProfileBindingRejectsZeroCreationTime(t *testing.T) {
	t.Parallel()
	if _, err := validNewApplicationProfileBinding(t).ApplicationProfileBinding(time.Time{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewApplicationProfileBinding(t *testing.T) NewApplicationProfileBinding {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewApplicationProfileBinding{
		TenantID: newID(), WorkspaceID: newID(), Name: "corporate-browser",
		Kind: ApplicationProfileBindingBrowser, Ref: "profile://browser/alice/payments",
	}
}
