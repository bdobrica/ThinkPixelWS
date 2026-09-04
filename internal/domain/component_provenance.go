package domain

import (
	"errors"
	"math"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

const (
	maxProvenanceSourceTypeLength = 64
	maxProvenancePrincipalLength  = 256
	maxProvenanceDerivedFrom      = 256
	maxProvenanceTaintLength      = 128
)

type ProvenanceClassification string
type ProvenanceSourceTrust string

const (
	ProvenancePublic       ProvenanceClassification = "public"
	ProvenanceInternal     ProvenanceClassification = "internal"
	ProvenanceConfidential ProvenanceClassification = "confidential"
	ProvenanceRestricted   ProvenanceClassification = "restricted"

	ProvenanceTrustedInternal       ProvenanceSourceTrust = "trusted-internal"
	ProvenanceAuthenticatedExternal ProvenanceSourceTrust = "authenticated-external"
	ProvenanceExternalUntrusted     ProvenanceSourceTrust = "external-untrusted"
)

// ComponentProvenance is immutable historical evidence for one component in
// one Workspace generation. It describes origin and trust; it grants no access
// to the source or to the initiating Run.
type ComponentProvenance struct {
	TenantID            uuid.UUID
	WorkspaceID         uuid.UUID
	Generation          uint64
	ComponentID         uuid.UUID
	Source              string
	SourceType          string
	SourceRevision      string
	ImportedAt          time.Time
	InitiatingPrincipal string
	SourceRunID         *uuid.UUID
	DerivedFrom         []shared.SHA256Digest
	Classification      ProvenanceClassification
	SourceTrust         ProvenanceSourceTrust
	Taints              []string
}

type NewComponentProvenance struct {
	TenantID            uuid.UUID
	WorkspaceID         uuid.UUID
	Generation          uint64
	ComponentID         uuid.UUID
	Source              string
	SourceType          string
	SourceRevision      string
	InitiatingPrincipal string
	SourceRunID         *uuid.UUID
	DerivedFrom         []shared.SHA256Digest
	Classification      ProvenanceClassification
	SourceTrust         ProvenanceSourceTrust
	Taints              []string
}

func (input NewComponentProvenance) ComponentProvenance(importedAt time.Time) (ComponentProvenance, error) {
	provenance := ComponentProvenance{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		Generation: input.Generation, ComponentID: input.ComponentID,
		Source: input.Source, SourceType: input.SourceType,
		SourceRevision: input.SourceRevision, ImportedAt: importedAt.UTC(),
		InitiatingPrincipal: input.InitiatingPrincipal, SourceRunID: input.SourceRunID,
		DerivedFrom:    append([]shared.SHA256Digest(nil), input.DerivedFrom...),
		Classification: input.Classification, SourceTrust: input.SourceTrust,
		Taints: append([]string(nil), input.Taints...),
	}
	if err := provenance.Validate(); err != nil {
		return ComponentProvenance{}, err
	}
	return provenance, nil
}

func (provenance ComponentProvenance) Validate() error {
	if provenance.TenantID == uuid.Nil || provenance.WorkspaceID == uuid.Nil || provenance.ComponentID == uuid.Nil {
		return errors.New("component provenance tenant, workspace, and component IDs are required")
	}
	if provenance.TenantID.Version() != 7 || provenance.WorkspaceID.Version() != 7 || provenance.ComponentID.Version() != 7 {
		return errors.New("component provenance tenant, workspace, and component IDs must be UUIDv7")
	}
	if provenance.Generation < 1 || provenance.Generation > math.MaxInt64 {
		return errors.New("component provenance generation is outside the supported range")
	}
	if !validSourceRef(provenance.Source) {
		return errors.New("component provenance source is invalid")
	}
	if !validBoundedSourceValue(provenance.SourceType, maxProvenanceSourceTypeLength) {
		return errors.New("component provenance source type is invalid")
	}
	if !validBoundedSourceValue(provenance.SourceRevision, maxSourceRevisionLength) {
		return errors.New("component provenance source revision is invalid")
	}
	if provenance.ImportedAt.IsZero() {
		return errors.New("component provenance import time is required")
	}
	if !validBoundedSourceValue(provenance.InitiatingPrincipal, maxProvenancePrincipalLength) {
		return errors.New("component provenance initiating principal is invalid")
	}
	if provenance.SourceRunID != nil && (*provenance.SourceRunID == uuid.Nil || provenance.SourceRunID.Version() != 7) {
		return errors.New("component provenance source Run ID must be UUIDv7")
	}
	if len(provenance.DerivedFrom) > maxProvenanceDerivedFrom {
		return errors.New("component provenance has too many derivation edges")
	}
	for _, digest := range provenance.DerivedFrom {
		if digest == (shared.SHA256Digest{}) {
			return errors.New("component provenance derivation digest is required")
		}
	}
	if !provenance.Classification.valid() {
		return errors.New("component provenance classification is invalid")
	}
	if !provenance.SourceTrust.valid() {
		return errors.New("component provenance source trust is invalid")
	}
	seenTaints := make(map[string]struct{}, len(provenance.Taints))
	for _, taint := range provenance.Taints {
		if !validBoundedSourceValue(taint, maxProvenanceTaintLength) {
			return errors.New("component provenance taint is invalid")
		}
		if _, exists := seenTaints[taint]; exists {
			return errors.New("component provenance taints must be unique")
		}
		seenTaints[taint] = struct{}{}
	}
	return nil
}

func (classification ProvenanceClassification) valid() bool {
	switch classification {
	case ProvenancePublic, ProvenanceInternal, ProvenanceConfidential, ProvenanceRestricted:
		return true
	default:
		return false
	}
}

func (trust ProvenanceSourceTrust) valid() bool {
	switch trust {
	case ProvenanceTrustedInternal, ProvenanceAuthenticatedExternal, ProvenanceExternalUntrusted:
		return true
	default:
		return false
	}
}
