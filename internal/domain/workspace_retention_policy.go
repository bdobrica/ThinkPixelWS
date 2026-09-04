package domain

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

const maxLegalHoldReferenceLength = 256

// WorkspaceRetentionPolicy records tenant-scoped lifecycle policy inputs. It
// does not itself authorize archive, deletion, transfer, or key destruction.
type WorkspaceRetentionPolicy struct {
	TenantID               uuid.UUID
	WorkspaceID            uuid.UUID
	StateVersion           uint64
	IdleTTL                *time.Duration
	ArchiveAfterInactivity *time.Duration
	SnapshotRetention      *time.Duration
	MaximumGenerations     *uint64
	DeleteAfter            *time.Duration
	ProfileRetention       *time.Duration
	LegalHold              bool
	LegalHoldReference     string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type NewWorkspaceRetentionPolicy struct {
	TenantID               uuid.UUID
	WorkspaceID            uuid.UUID
	IdleTTL                *time.Duration
	ArchiveAfterInactivity *time.Duration
	SnapshotRetention      *time.Duration
	MaximumGenerations     *uint64
	DeleteAfter            *time.Duration
	ProfileRetention       *time.Duration
	LegalHold              bool
	LegalHoldReference     string
}

func (input NewWorkspaceRetentionPolicy) WorkspaceRetentionPolicy(now time.Time) (WorkspaceRetentionPolicy, error) {
	policy := WorkspaceRetentionPolicy{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID, StateVersion: 1,
		IdleTTL: cloneDuration(input.IdleTTL), ArchiveAfterInactivity: cloneDuration(input.ArchiveAfterInactivity),
		SnapshotRetention: cloneDuration(input.SnapshotRetention), MaximumGenerations: cloneUint64(input.MaximumGenerations),
		DeleteAfter: cloneDuration(input.DeleteAfter), ProfileRetention: cloneDuration(input.ProfileRetention),
		LegalHold: input.LegalHold, LegalHoldReference: input.LegalHoldReference,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if err := policy.Validate(); err != nil {
		return WorkspaceRetentionPolicy{}, err
	}
	return policy, nil
}

func (policy WorkspaceRetentionPolicy) Validate() error {
	if policy.TenantID == uuid.Nil || policy.WorkspaceID == uuid.Nil {
		return errors.New("retention policy tenant and Workspace IDs are required")
	}
	if policy.TenantID.Version() != 7 || policy.WorkspaceID.Version() != 7 {
		return errors.New("retention policy tenant and Workspace IDs must be UUIDv7")
	}
	if policy.StateVersion < 1 || policy.StateVersion > math.MaxInt64 {
		return errors.New("retention policy state version is outside the supported range")
	}
	for _, duration := range []*time.Duration{policy.IdleTTL, policy.ArchiveAfterInactivity, policy.SnapshotRetention, policy.DeleteAfter, policy.ProfileRetention} {
		if duration != nil && (*duration <= 0 || *duration%time.Second != 0) {
			return errors.New("retention policy durations must be positive whole seconds when set")
		}
	}
	if policy.MaximumGenerations != nil && (*policy.MaximumGenerations < 1 || *policy.MaximumGenerations > math.MaxInt64) {
		return errors.New("retention policy maximum generations is outside the supported range")
	}
	if policy.LegalHold {
		if !validBoundedSourceValue(policy.LegalHoldReference, maxLegalHoldReferenceLength) {
			return errors.New("legal hold reference is required and must be valid while held")
		}
	} else if policy.LegalHoldReference != "" {
		return errors.New("legal hold reference must be empty while not held")
	}
	if policy.CreatedAt.IsZero() || policy.UpdatedAt.Before(policy.CreatedAt) {
		return errors.New("retention policy timestamps are invalid")
	}
	return nil
}

func cloneDuration(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
