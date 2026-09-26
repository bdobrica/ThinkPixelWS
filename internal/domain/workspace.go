package domain

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxWorkspaceDescriptionLength = 4096
	maxOwnerIDLength              = 256
	maxResidencyLabels            = 64
	maxResidencyLabelLength       = 63
)

var workspaceNamePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

var (
	ErrInvalidWorkspaceStateTransition = errors.New("invalid workspace state transition")
	ErrWorkspaceStateVersionConflict   = errors.New("workspace state version conflict")
	ErrWorkspaceWriterFenceExhausted   = errors.New("workspace writer fence exhausted")
)

type WorkspaceState string

const (
	WorkspaceCreating  WorkspaceState = "CREATING"
	WorkspaceReady     WorkspaceState = "READY"
	WorkspaceArchiving WorkspaceState = "ARCHIVING"
	WorkspaceArchived  WorkspaceState = "ARCHIVED"
	WorkspaceRestoring WorkspaceState = "RESTORING"
	WorkspaceDeleting  WorkspaceState = "DELETING"
	WorkspaceDeleted   WorkspaceState = "DELETED"
	WorkspaceDegraded  WorkspaceState = "DEGRADED"
)

type OwnerKind string

const (
	OwnerUser    OwnerKind = "user"
	OwnerTeam    OwnerKind = "team"
	OwnerService OwnerKind = "service"
	OwnerProject OwnerKind = "project"
)

type Classification string

const (
	ClassificationPublic       Classification = "public"
	ClassificationInternal     Classification = "internal"
	ClassificationConfidential Classification = "confidential"
	ClassificationRestricted   Classification = "restricted"
)

type Owner struct {
	Kind OwnerKind
	ID   string
}

// Workspace is a tenant-scoped logical identity. Its owner and residency
// metadata are context for policy evaluation and never establish authority.
type Workspace struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	Name           string
	Description    string
	Owner          Owner
	State          WorkspaceState
	StateVersion   uint64
	WriterFence    uint64
	Classification Classification
	Residency      []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type NewWorkspace struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	Name           string
	Description    string
	Owner          Owner
	Classification Classification
	Residency      []string
}

func (input NewWorkspace) Workspace(now time.Time) (Workspace, error) {
	workspace := Workspace{
		TenantID: input.TenantID, ID: input.ID, Name: input.Name,
		Description: input.Description, Owner: input.Owner,
		State: WorkspaceCreating, StateVersion: 1,
		Classification: input.Classification, Residency: append([]string(nil), input.Residency...),
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if workspace.Classification == "" {
		workspace.Classification = ClassificationInternal
	}
	if err := workspace.Validate(); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func (workspace Workspace) Validate() error {
	if workspace.TenantID == uuid.Nil || workspace.ID == uuid.Nil {
		return errors.New("tenant and workspace IDs are required")
	}
	if workspace.TenantID.Version() != 7 {
		return errors.New("tenant ID must be UUIDv7")
	}
	if workspace.ID.Version() != 7 {
		return errors.New("workspace ID must be UUIDv7")
	}
	if !workspaceNamePattern.MatchString(workspace.Name) {
		return errors.New("workspace name has invalid format")
	}
	if utf8.RuneCountInString(workspace.Description) > maxWorkspaceDescriptionLength {
		return errors.New("workspace description exceeds 4096 characters")
	}
	if !workspace.Owner.Kind.valid() {
		return errors.New("workspace owner kind is invalid")
	}
	if strings.TrimSpace(workspace.Owner.ID) != workspace.Owner.ID || workspace.Owner.ID == "" || utf8.RuneCountInString(workspace.Owner.ID) > maxOwnerIDLength {
		return errors.New("workspace owner ID is invalid")
	}
	if !workspace.State.valid() || workspace.StateVersion < 1 || workspace.StateVersion > math.MaxInt64 {
		return errors.New("workspace state is invalid")
	}
	if workspace.WriterFence > math.MaxInt64 {
		return errors.New("workspace writer fence exceeds supported range")
	}
	if !workspace.Classification.valid() {
		return errors.New("workspace classification is invalid")
	}
	if len(workspace.Residency) > maxResidencyLabels {
		return fmt.Errorf("workspace residency exceeds %d labels", maxResidencyLabels)
	}
	seen := make(map[string]struct{}, len(workspace.Residency))
	for _, label := range workspace.Residency {
		if strings.TrimSpace(label) != label || label == "" || utf8.RuneCountInString(label) > maxResidencyLabelLength {
			return errors.New("workspace residency label is invalid")
		}
		if _, duplicate := seen[label]; duplicate {
			return errors.New("workspace residency labels must be unique")
		}
		seen[label] = struct{}{}
	}
	if workspace.CreatedAt.IsZero() || workspace.UpdatedAt.Before(workspace.CreatedAt) {
		return errors.New("workspace timestamps are invalid")
	}
	return nil
}

// TransitionState applies one accepted Workspace lifecycle edge. The expected
// version makes callers acknowledge the state they observed before persisting
// the transition with a repository compare-and-swap.
func (workspace Workspace) TransitionState(next WorkspaceState, expectedVersion uint64, now time.Time) (Workspace, error) {
	if expectedVersion != workspace.StateVersion {
		return Workspace{}, ErrWorkspaceStateVersionConflict
	}
	if !workspace.State.CanTransitionTo(next) {
		return Workspace{}, fmt.Errorf("%w: %s to %s", ErrInvalidWorkspaceStateTransition, workspace.State, next)
	}
	if workspace.StateVersion >= math.MaxInt64 {
		return Workspace{}, errors.New("workspace state version exhausted")
	}
	now = now.UTC()
	if now.Before(workspace.UpdatedAt) {
		return Workspace{}, errors.New("workspace transition time precedes last update")
	}

	workspace.State = next
	workspace.StateVersion++
	workspace.UpdatedAt = now
	return workspace, nil
}

func (state WorkspaceState) CanTransitionTo(next WorkspaceState) bool {
	switch state {
	case WorkspaceCreating:
		return next == WorkspaceReady || next == WorkspaceDegraded
	case WorkspaceDegraded:
		return next == WorkspaceReady
	case WorkspaceReady:
		return next == WorkspaceArchiving || next == WorkspaceDeleting
	case WorkspaceArchiving:
		return next == WorkspaceArchived || next == WorkspaceDegraded
	case WorkspaceArchived:
		return next == WorkspaceRestoring || next == WorkspaceDeleting
	case WorkspaceRestoring:
		return next == WorkspaceReady || next == WorkspaceDegraded
	case WorkspaceDeleting:
		return next == WorkspaceDeleted
	default:
		return false
	}
}

func (kind OwnerKind) valid() bool {
	switch kind {
	case OwnerUser, OwnerTeam, OwnerService, OwnerProject:
		return true
	default:
		return false
	}
}

func (state WorkspaceState) valid() bool {
	switch state {
	case WorkspaceCreating, WorkspaceReady, WorkspaceArchiving, WorkspaceArchived,
		WorkspaceRestoring, WorkspaceDeleting, WorkspaceDeleted, WorkspaceDegraded:
		return true
	default:
		return false
	}
}

func (classification Classification) valid() bool {
	switch classification {
	case ClassificationPublic, ClassificationInternal, ClassificationConfidential, ClassificationRestricted:
		return true
	default:
		return false
	}
}
