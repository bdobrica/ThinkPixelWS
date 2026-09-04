package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type ExternalBindingKind string
type ExternalBindingMode string
type ExternalBindingClassification string

const (
	ExternalBindingCollaboration ExternalBindingKind = "collaboration"
	ExternalBindingIssueTracker  ExternalBindingKind = "issue-tracker"
	ExternalBindingDocumentStore ExternalBindingKind = "document-store"
	ExternalBindingDatabase      ExternalBindingKind = "database"
	ExternalBindingService       ExternalBindingKind = "service"
	ExternalBindingObservability ExternalBindingKind = "observability"

	ExternalBindingLiveReference    ExternalBindingMode = "live-reference"
	ExternalBindingMaterializedCopy ExternalBindingMode = "materialized-copy"

	ExternalBindingPublic       ExternalBindingClassification = "public"
	ExternalBindingInternal     ExternalBindingClassification = "internal"
	ExternalBindingConfidential ExternalBindingClassification = "confidential"
	ExternalBindingRestricted   ExternalBindingClassification = "restricted"
)

// ExternalBinding describes a credential-free external context reference.
// It is descriptive metadata and does not grant access to the resource.
type ExternalBinding struct {
	TenantID       uuid.UUID
	WorkspaceID    uuid.UUID
	Name           string
	Kind           ExternalBindingKind
	Ref            string
	Mode           ExternalBindingMode
	Classification *ExternalBindingClassification
	CreatedAt      time.Time
}

type NewExternalBinding struct {
	TenantID       uuid.UUID
	WorkspaceID    uuid.UUID
	Name           string
	Kind           ExternalBindingKind
	Ref            string
	Mode           ExternalBindingMode
	Classification *ExternalBindingClassification
}

func (input NewExternalBinding) ExternalBinding(now time.Time) (ExternalBinding, error) {
	binding := ExternalBinding{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID, Name: input.Name,
		Kind: input.Kind, Ref: input.Ref, Mode: input.Mode,
		Classification: input.Classification, CreatedAt: now.UTC(),
	}
	if err := binding.Validate(); err != nil {
		return ExternalBinding{}, err
	}
	return binding, nil
}

func (binding ExternalBinding) Validate() error {
	if binding.TenantID == uuid.Nil || binding.WorkspaceID == uuid.Nil {
		return errors.New("external binding tenant and workspace IDs are required")
	}
	if binding.TenantID.Version() != 7 || binding.WorkspaceID.Version() != 7 {
		return errors.New("external binding tenant and workspace IDs must be UUIDv7")
	}
	if !workspaceNamePattern.MatchString(binding.Name) {
		return errors.New("external binding name has invalid format")
	}
	if !binding.Kind.valid() {
		return errors.New("external binding kind is invalid")
	}
	if !validSourceRef(binding.Ref) {
		return errors.New("external binding ref is invalid")
	}
	if !binding.Mode.valid() {
		return errors.New("external binding mode is invalid")
	}
	if binding.Classification != nil && !binding.Classification.valid() {
		return errors.New("external binding classification is invalid")
	}
	if binding.CreatedAt.IsZero() {
		return errors.New("external binding creation time is required")
	}
	return nil
}

func (kind ExternalBindingKind) valid() bool {
	switch kind {
	case ExternalBindingCollaboration, ExternalBindingIssueTracker,
		ExternalBindingDocumentStore, ExternalBindingDatabase,
		ExternalBindingService, ExternalBindingObservability:
		return true
	default:
		return false
	}
}

func (mode ExternalBindingMode) valid() bool {
	return mode == ExternalBindingLiveReference || mode == ExternalBindingMaterializedCopy
}

func (classification ExternalBindingClassification) valid() bool {
	switch classification {
	case ExternalBindingPublic, ExternalBindingInternal,
		ExternalBindingConfidential, ExternalBindingRestricted:
		return true
	default:
		return false
	}
}
