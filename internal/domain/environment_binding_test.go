package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

func TestNewEnvironmentBinding(t *testing.T) {
	t.Parallel()
	digest := shared.DigestBytes([]byte("environment definition"))
	platform := "linux/amd64"
	input := validNewEnvironmentBinding(t)
	input.Digest = &digest
	input.Platform = &platform
	now := time.Date(2026, time.September, 4, 18, 0, 0, 0, time.FixedZone("test", 3*60*60))
	binding, err := input.EnvironmentBinding(now)
	if err != nil {
		t.Fatalf("create environment binding: %v", err)
	}
	if binding.Kind != input.Kind || binding.Ref != input.Ref ||
		binding.Digest == nil || *binding.Digest != digest ||
		binding.Platform == nil || *binding.Platform != platform ||
		binding.CreatedAt.Location() != time.UTC || !binding.CreatedAt.Equal(now) {
		t.Fatalf("unexpected environment binding: %#v", binding)
	}
}

func TestNewEnvironmentBindingAllowsOptionalMetadataToBeOmitted(t *testing.T) {
	t.Parallel()
	if _, err := validNewEnvironmentBinding(t).EnvironmentBinding(time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestNewEnvironmentBindingAcceptsContractKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []EnvironmentBindingKind{
		EnvironmentBindingOCI, EnvironmentBindingThinkPixelMP,
		EnvironmentBindingDevContainer, EnvironmentBindingDevfile,
		EnvironmentBindingRuntimeProfile,
	} {
		input := validNewEnvironmentBinding(t)
		input.Kind = kind
		if _, err := input.EnvironmentBinding(time.Now()); err != nil {
			t.Fatalf("kind %q: %v", kind, err)
		}
	}
}

func TestNewEnvironmentBindingRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*NewEnvironmentBinding){
		"non-v7 tenant ID":    func(input *NewEnvironmentBinding) { input.TenantID = uuid.New() },
		"non-v7 workspace ID": func(input *NewEnvironmentBinding) { input.WorkspaceID = uuid.New() },
		"unknown kind":        func(input *NewEnvironmentBinding) { input.Kind = "dockerfile" },
		"relative ref":        func(input *NewEnvironmentBinding) { input.Ref = "environments/go" },
		"credential-bearing ref": func(input *NewEnvironmentBinding) {
			input.Ref = "https://user:secret@example.test/environment"
		},
		"control in ref": func(input *NewEnvironmentBinding) { input.Ref = "oci://registry/image\n" },
		"long ref": func(input *NewEnvironmentBinding) {
			input.Ref = "urn:" + strings.Repeat("a", maxSourceRefLength)
		},
		"empty platform": func(input *NewEnvironmentBinding) { value := ""; input.Platform = &value },
		"long platform": func(input *NewEnvironmentBinding) {
			value := strings.Repeat("a", maxEnvironmentPlatformLength+1)
			input.Platform = &value
		},
		"control in platform": func(input *NewEnvironmentBinding) {
			value := "linux/amd64\n"
			input.Platform = &value
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewEnvironmentBinding(t)
			mutate(&input)
			if _, err := input.EnvironmentBinding(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewEnvironmentBindingRejectsZeroCreationTime(t *testing.T) {
	t.Parallel()
	if _, err := validNewEnvironmentBinding(t).EnvironmentBinding(time.Time{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewEnvironmentBinding(t *testing.T) NewEnvironmentBinding {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewEnvironmentBinding{
		TenantID: newID(), WorkspaceID: newID(), Kind: EnvironmentBindingOCI,
		Ref: "oci://registry.example.test/engineering@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}
