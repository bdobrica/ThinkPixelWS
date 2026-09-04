package domain

import (
	"errors"
	"fmt"
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
	if !workspace.State.valid() || workspace.StateVersion < 1 {
		return errors.New("workspace state is invalid")
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
