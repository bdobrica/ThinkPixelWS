package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

// GenerationComponentReference identifies captured immutable content, never a
// mutable working volume. For portable snapshots Ref is the snapshot manifest's
// SHA-256 digest; ComponentID selects its component. For provider checkpoints,
// Provider and TargetID scope the opaque immutable checkpoint Ref (e.g. a UID,
// not a reusable snapshot name). References contain no credentials or authority.
// The trusted capture caller verifies existence, immutability and manifest match.
type GenerationComponentReference struct {
	ComponentID uuid.UUID `json:"componentId"`
	Kind        string    `json:"kind"`
	Ref         string    `json:"ref"`
	Provider    string    `json:"provider,omitempty"`
	TargetID    string    `json:"targetId,omitempty"`
}

const (
	GenerationReferencePortableSnapshot   = "portable-snapshot"
	GenerationReferenceProviderCheckpoint = "provider-checkpoint"
)

func validateGenerationComponentReferences(refs []GenerationComponentReference, durability GenerationDurability) error {
	if len(refs) > 256 {
		return errors.New("generation has too many component references")
	}
	seen := make(map[uuid.UUID]bool, len(refs))
	for _, ref := range refs {
		if ref.ComponentID == uuid.Nil || ref.ComponentID.Version() != 7 || seen[ref.ComponentID] {
			return errors.New("generation component IDs must be unique UUIDv7 values")
		}
		seen[ref.ComponentID] = true
		switch ref.Kind {
		case GenerationReferencePortableSnapshot:
			digest, err := shared.ParseSHA256Digest(ref.Ref)
			if err != nil || digest == (shared.SHA256Digest{}) || ref.Provider != "" || ref.TargetID != "" {
				return errors.New("portable component reference requires only an immutable snapshot digest")
			}
		case GenerationReferenceProviderCheckpoint:
			if durability == GenerationDurabilityPortable {
				return errors.New("portable generation cannot depend on a provider checkpoint")
			}
			if !validGenerationReferenceText(ref.Ref, 4096) || !validGenerationReferenceText(ref.Provider, 128) || !validGenerationReferenceText(ref.TargetID, 256) {
				return errors.New("checkpoint component reference requires provider, target and immutable handle")
			}
		default:
			return errors.New("generation component reference kind is invalid")
		}
	}
	return nil
}

func validGenerationReferenceText(value string, max int) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= max && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}
