package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type ApplicationProfileBindingKind string

const (
	ApplicationProfileBindingBrowser     ApplicationProfileBindingKind = "browser"
	ApplicationProfileBindingDesktop     ApplicationProfileBindingKind = "desktop"
	ApplicationProfileBindingApplication ApplicationProfileBindingKind = "application"
	ApplicationProfileBindingIDE         ApplicationProfileBindingKind = "ide"

	maxProfileRetentionPolicyLength = 128
)

// ApplicationProfileBinding identifies credential-adjacent application state
// managed outside the Workspace by a ProfileProvider. The reference conveys no
// authority to resolve or materialize the profile.
type ApplicationProfileBinding struct {
	TenantID        uuid.UUID
	WorkspaceID     uuid.UUID
	Name            string
	Kind            ApplicationProfileBindingKind
	Ref             string
	RetentionPolicy *string
	CreatedAt       time.Time
}

type NewApplicationProfileBinding struct {
	TenantID        uuid.UUID
	WorkspaceID     uuid.UUID
	Name            string
	Kind            ApplicationProfileBindingKind
	Ref             string
	RetentionPolicy *string
}

func (input NewApplicationProfileBinding) ApplicationProfileBinding(now time.Time) (ApplicationProfileBinding, error) {
	binding := ApplicationProfileBinding{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID, Name: input.Name,
		Kind: input.Kind, Ref: input.Ref, RetentionPolicy: input.RetentionPolicy,
		CreatedAt: now.UTC(),
	}
	if err := binding.Validate(); err != nil {
		return ApplicationProfileBinding{}, err
	}
	return binding, nil
}

func (binding ApplicationProfileBinding) Validate() error {
	if binding.TenantID == uuid.Nil || binding.WorkspaceID == uuid.Nil {
		return errors.New("application profile binding tenant and workspace IDs are required")
	}
	if binding.TenantID.Version() != 7 || binding.WorkspaceID.Version() != 7 {
		return errors.New("application profile binding tenant and workspace IDs must be UUIDv7")
	}
	if !workspaceNamePattern.MatchString(binding.Name) {
		return errors.New("application profile binding name has invalid format")
	}
	if !binding.Kind.valid() {
		return errors.New("application profile binding kind is invalid")
	}
	if !validSourceRef(binding.Ref) {
		return errors.New("application profile binding ref is invalid")
	}
	if binding.RetentionPolicy != nil && !validBoundedSourceValue(*binding.RetentionPolicy, maxProfileRetentionPolicyLength) {
		return errors.New("application profile binding retention policy is invalid")
	}
	if binding.CreatedAt.IsZero() {
		return errors.New("application profile binding creation time is required")
	}
	return nil
}

func (kind ApplicationProfileBindingKind) valid() bool {
	switch kind {
	case ApplicationProfileBindingBrowser, ApplicationProfileBindingDesktop,
		ApplicationProfileBindingApplication, ApplicationProfileBindingIDE:
		return true
	default:
		return false
	}
}
