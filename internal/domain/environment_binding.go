package domain

import (
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

type EnvironmentBindingKind string

const (
	EnvironmentBindingOCI            EnvironmentBindingKind = "oci"
	EnvironmentBindingThinkPixelMP   EnvironmentBindingKind = "thinkpixelmp"
	EnvironmentBindingDevContainer   EnvironmentBindingKind = "devcontainer"
	EnvironmentBindingDevfile        EnvironmentBindingKind = "devfile"
	EnvironmentBindingRuntimeProfile EnvironmentBindingKind = "runtime-profile"

	maxEnvironmentPlatformLength = 128
)

// EnvironmentBinding identifies a reproducible execution-environment
// definition or qualified artifact. Its fields are untrusted requirements
// metadata and convey no runtime privilege or authority.
type EnvironmentBinding struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	Kind        EnvironmentBindingKind
	Ref         string
	Digest      *shared.SHA256Digest
	Platform    *string
	CreatedAt   time.Time
}

type NewEnvironmentBinding struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	Kind        EnvironmentBindingKind
	Ref         string
	Digest      *shared.SHA256Digest
	Platform    *string
}

func (input NewEnvironmentBinding) EnvironmentBinding(now time.Time) (EnvironmentBinding, error) {
	binding := EnvironmentBinding{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		Kind: input.Kind, Ref: input.Ref, Digest: input.Digest,
		Platform: input.Platform, CreatedAt: now.UTC(),
	}
	if err := binding.Validate(); err != nil {
		return EnvironmentBinding{}, err
	}
	return binding, nil
}

func (binding EnvironmentBinding) Validate() error {
	if binding.TenantID == uuid.Nil || binding.WorkspaceID == uuid.Nil {
		return errors.New("environment binding tenant and workspace IDs are required")
	}
	if binding.TenantID.Version() != 7 || binding.WorkspaceID.Version() != 7 {
		return errors.New("environment binding tenant and workspace IDs must be UUIDv7")
	}
	if !binding.Kind.valid() {
		return errors.New("environment binding kind is invalid")
	}
	if !validSourceRef(binding.Ref) {
		return errors.New("environment binding ref is invalid")
	}
	if binding.Platform != nil && !validBoundedSourceValue(*binding.Platform, maxEnvironmentPlatformLength) {
		return errors.New("environment binding platform is invalid")
	}
	if binding.CreatedAt.IsZero() {
		return errors.New("environment binding creation time is required")
	}
	return nil
}

func (kind EnvironmentBindingKind) valid() bool {
	switch kind {
	case EnvironmentBindingOCI, EnvironmentBindingThinkPixelMP,
		EnvironmentBindingDevContainer, EnvironmentBindingDevfile,
		EnvironmentBindingRuntimeProfile:
		return true
	default:
		return false
	}
}
