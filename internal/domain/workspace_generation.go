package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

const maxGenerationCreatorIDLength = 256

type WorkspaceGenerationState string

const WorkspaceGenerationCompleted WorkspaceGenerationState = "COMPLETED"

type GenerationDurability string

const (
	GenerationDurabilityProviderLocal GenerationDurability = "provider-local"
	GenerationDurabilityPortable      GenerationDurability = "portable"
)

// WorkspaceGeneration identifies one immutable, committed logical state of a
// Workspace. Component and provenance records are introduced separately.
type WorkspaceGeneration struct {
	TenantID           uuid.UUID
	WorkspaceID        uuid.UUID
	ID                 uuid.UUID
	Number             uint64
	ParentNumber       *uint64
	State              WorkspaceGenerationState
	ManifestDigest     shared.SHA256Digest
	Durability         GenerationDurability
	CreatedByPrincipal string
	CreatedByRun       *uuid.UUID
	CreatedByExecution *uuid.UUID
	CreatedAt          time.Time
}

type NewWorkspaceGeneration struct {
	TenantID           uuid.UUID
	WorkspaceID        uuid.UUID
	ID                 uuid.UUID
	Number             uint64
	ParentNumber       *uint64
	ManifestDigest     shared.SHA256Digest
	Durability         GenerationDurability
	CreatedByPrincipal string
	CreatedByRun       *uuid.UUID
	CreatedByExecution *uuid.UUID
}

func (input NewWorkspaceGeneration) WorkspaceGeneration(now time.Time) (WorkspaceGeneration, error) {
	generation := WorkspaceGeneration{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID, ID: input.ID,
		Number: input.Number, ParentNumber: input.ParentNumber,
		State: WorkspaceGenerationCompleted, ManifestDigest: input.ManifestDigest,
		Durability: input.Durability, CreatedByPrincipal: input.CreatedByPrincipal,
		CreatedByRun:       cloneUUID(input.CreatedByRun),
		CreatedByExecution: cloneUUID(input.CreatedByExecution),
		CreatedAt:          now.UTC(),
	}
	if err := generation.Validate(); err != nil {
		return WorkspaceGeneration{}, err
	}
	return generation, nil
}

func (generation WorkspaceGeneration) Validate() error {
	if generation.TenantID == uuid.Nil || generation.WorkspaceID == uuid.Nil || generation.ID == uuid.Nil {
		return errors.New("tenant, workspace, and generation IDs are required")
	}
	if generation.TenantID.Version() != 7 || generation.WorkspaceID.Version() != 7 || generation.ID.Version() != 7 {
		return errors.New("tenant, workspace, and generation IDs must be UUIDv7")
	}
	if generation.Number < 1 || generation.Number > math.MaxInt64 {
		return errors.New("generation number is outside the supported range")
	}
	if generation.ParentNumber != nil && (*generation.ParentNumber < 1 || *generation.ParentNumber >= generation.Number) {
		return errors.New("parent generation must precede the generation")
	}
	if generation.State != WorkspaceGenerationCompleted {
		return errors.New("workspace generation state is invalid")
	}
	if generation.ManifestDigest == (shared.SHA256Digest{}) {
		return errors.New("workspace generation manifest digest is required")
	}
	if generation.Durability != GenerationDurabilityProviderLocal && generation.Durability != GenerationDurabilityPortable {
		return errors.New("workspace generation durability is invalid")
	}
	if strings.TrimSpace(generation.CreatedByPrincipal) != generation.CreatedByPrincipal || generation.CreatedByPrincipal == "" || utf8.RuneCountInString(generation.CreatedByPrincipal) > maxGenerationCreatorIDLength {
		return errors.New("workspace generation creator is invalid")
	}
	if !validOptionalUUIDv7(generation.CreatedByRun) {
		return errors.New("workspace generation Run ID must be UUIDv7")
	}
	if generation.CreatedByExecution != nil && (generation.CreatedByExecution.Version() != 7 || *generation.CreatedByExecution == uuid.Nil) {
		return errors.New("workspace generation execution ID must be UUIDv7")
	}
	if generation.CreatedAt.IsZero() {
		return errors.New("workspace generation creation time is required")
	}
	return nil
}
