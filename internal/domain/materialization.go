package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type MaterializationMode string

const (
	MaterializationReadOnly  MaterializationMode = "read-only"
	MaterializationReadWrite MaterializationMode = "read-write"
)

type MaterializationState string

const (
	MaterializationRequested     MaterializationState = "REQUESTED"
	MaterializationPreparing     MaterializationState = "PREPARING"
	MaterializationReady         MaterializationState = "READY"
	MaterializationActive        MaterializationState = "ACTIVE"
	MaterializationCheckpointing MaterializationState = "CHECKPOINTING"
	MaterializationReleasing     MaterializationState = "RELEASING"
	MaterializationReleased      MaterializationState = "RELEASED"
	MaterializationFailed        MaterializationState = "FAILED"
	MaterializationFenced        MaterializationState = "FENCED"
)

// MaterializationTarget describes requested placement, not permission to use it.
// Provider binding handles and credentials are deliberately separate.
type MaterializationTarget struct {
	ID           string
	Region       string
	StorageClass string
	Architecture string
}

// MaterializationHandle is an opaque, non-authorizing provider reference.
// Trusted adapters must supply references only, never credentials or grants.
// Consumers must not interpret it as a path, URL, or public Workspace identity.
type MaterializationHandle string

func (h MaterializationHandle) Validate() error {
	if !utf8.ValidString(string(h)) || !validBoundedSourceValue(string(h), 4096) {
		return errors.New("materialization handle is invalid")
	}
	return nil
}

// Materialization records a temporary realization of one completed generation.
// A read-write record is only intent: it does not establish a writer lease,
// execution authority, or permission to provision or attach storage.
type Materialization struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	BaseGeneration uint64
	// CleanGeneration is zero when cleanliness is unknown. A nonzero value
	// records a quiesced capture while CHECKPOINTING; it is not writer authority
	// or proof of current Workspace head. Resume writes only after leaving that state.
	CleanGeneration uint64
	Provider        string
	Target          MaterializationTarget
	Handle          MaterializationHandle
	Mode            MaterializationMode
	State           MaterializationState
	StateVersion    uint64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type NewMaterialization struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	BaseGeneration uint64
	Provider       string
	Target         MaterializationTarget
	Mode           MaterializationMode
}

func (input NewMaterialization) Materialization(now time.Time) (Materialization, error) {
	m := Materialization{
		TenantID: input.TenantID, ID: input.ID, WorkspaceID: input.WorkspaceID,
		BaseGeneration: input.BaseGeneration, Provider: input.Provider, Target: input.Target,
		Mode: input.Mode, State: MaterializationRequested, StateVersion: 1,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if err := m.Validate(); err != nil {
		return Materialization{}, err
	}
	return m, nil
}

func (m Materialization) Validate() error {
	if m.CleanGeneration != 0 && (m.CleanGeneration > math.MaxInt64 || m.Mode != MaterializationReadWrite || m.State != MaterializationCheckpointing) {
		return errors.New("materialization clean generation requires a writable checkpointing materialization and valid generation")
	}
	if m.Handle != "" {
		if err := m.Handle.Validate(); err != nil {
			return err
		}
		if m.State == MaterializationRequested {
			return errors.New("requested materialization cannot have a provider handle")
		}
	}
	for _, id := range []uuid.UUID{m.TenantID, m.ID, m.WorkspaceID} {
		if id.Version() != 7 || id.Variant() != uuid.RFC4122 {
			return errors.New("materialization IDs must be UUIDv7")
		}
	}
	if m.BaseGeneration < 1 || m.BaseGeneration > math.MaxInt64 {
		return errors.New("materialization base generation is invalid")
	}
	if !sourceProviderPattern.MatchString(m.Provider) {
		return errors.New("materialization provider is invalid")
	}
	if !validBoundedSourceValue(m.Target.ID, 256) || !validBoundedSourceValue(m.Target.Region, 63) || !validBoundedSourceValue(m.Target.StorageClass, 253) ||
		(m.Target.Architecture != "" && !validBoundedSourceValue(m.Target.Architecture, 32)) {
		return errors.New("materialization target is invalid")
	}
	if m.Mode != MaterializationReadOnly && m.Mode != MaterializationReadWrite {
		return errors.New("materialization mode is invalid")
	}
	switch m.State {
	case MaterializationRequested, MaterializationPreparing, MaterializationReady, MaterializationActive,
		MaterializationCheckpointing, MaterializationReleasing, MaterializationReleased, MaterializationFailed, MaterializationFenced:
	default:
		return errors.New("materialization state is invalid")
	}
	if m.StateVersion < 1 || m.StateVersion > math.MaxInt64 {
		return errors.New("materialization state version is invalid")
	}
	if m.CreatedAt.IsZero() || m.UpdatedAt.Before(m.CreatedAt) {
		return errors.New("materialization timestamps are invalid")
	}
	return nil
}

var (
	ErrMaterializationBindingConflict        = errors.New("materialization binding conflict")
	ErrInvalidMaterializationStateTransition = errors.New("invalid materialization state transition")
	ErrMaterializationStateVersionConflict   = errors.New("materialization state version conflict")
)

// Bind records the result of provider preparation once. A replacement needs a
// new Materialization; the handle remains available for cleanup after termination.
// Persistence must compare the observed version, and authority is checked separately.
func (m Materialization) Bind(handle MaterializationHandle, expectedVersion uint64, now time.Time) (Materialization, error) {
	if err := m.Validate(); err != nil {
		return Materialization{}, err
	}
	if expectedVersion != m.StateVersion {
		return Materialization{}, ErrMaterializationStateVersionConflict
	}
	if m.State != MaterializationPreparing || m.Handle != "" {
		return Materialization{}, ErrMaterializationBindingConflict
	}
	if err := handle.Validate(); err != nil {
		return Materialization{}, err
	}
	if m.StateVersion >= math.MaxInt64 || now.IsZero() || now.Before(m.UpdatedAt) {
		return Materialization{}, errors.New("invalid materialization binding version or time")
	}
	m.Handle = handle
	m.StateVersion++
	m.UpdatedAt = now.UTC()
	return m, nil
}

// CanTransitionTo describes lifecycle edges only, never permission to execute
// or write. Any nonterminal writer can be fenced when its lease expires, including
// during preparation. Released, failed, and fenced instances cannot be revived.
func (state MaterializationState) CanTransitionTo(next MaterializationState) bool {
	switch state {
	case MaterializationRequested:
		return next == MaterializationPreparing || next == MaterializationFailed || next == MaterializationFenced
	case MaterializationPreparing:
		return next == MaterializationReady || next == MaterializationFailed || next == MaterializationFenced
	case MaterializationReady:
		return next == MaterializationActive || next == MaterializationReleasing || next == MaterializationFenced
	case MaterializationActive:
		return next == MaterializationCheckpointing || next == MaterializationReleasing || next == MaterializationFenced
	case MaterializationCheckpointing:
		return next == MaterializationActive || next == MaterializationFailed || next == MaterializationFenced
	case MaterializationReleasing:
		return next == MaterializationReleased || next == MaterializationFailed || next == MaterializationFenced
	default:
		return false
	}
}

// TransitionState applies a lifecycle edge to a copy. Persistence must compare
// the observed state/version; callers must separately enforce authority and leases.
func (m Materialization) TransitionState(next MaterializationState, expectedVersion uint64, now time.Time) (Materialization, error) {
	if err := m.Validate(); err != nil {
		return Materialization{}, err
	}
	if expectedVersion != m.StateVersion {
		return Materialization{}, ErrMaterializationStateVersionConflict
	}
	if !m.State.CanTransitionTo(next) {
		return Materialization{}, fmt.Errorf("%w: %s to %s", ErrInvalidMaterializationStateTransition, m.State, next)
	}
	if m.StateVersion >= math.MaxInt64 {
		return Materialization{}, errors.New("materialization state version exhausted")
	}
	if now.IsZero() || now.Before(m.UpdatedAt) {
		return Materialization{}, errors.New("materialization transition time precedes last update")
	}
	m.State = next
	m.CleanGeneration = 0
	m.StateVersion++
	m.UpdatedAt = now.UTC()
	return m, nil
}
