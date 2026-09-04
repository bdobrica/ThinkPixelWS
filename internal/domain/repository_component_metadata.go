package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxVersionControlSystemLength = 64
	maxRepositoryDefaultRefLength = 512
)

var versionControlSystemPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// RepositoryComponentMetadata describes repository-specific behavior without
// duplicating source bindings, resolved revisions, provenance, or credentials.
type RepositoryComponentMetadata struct {
	TenantID             uuid.UUID
	WorkspaceID          uuid.UUID
	ComponentID          uuid.UUID
	VersionControlSystem string
	DefaultRef           *string
	CreatedAt            time.Time
}

type NewRepositoryComponentMetadata struct {
	TenantID             uuid.UUID
	WorkspaceID          uuid.UUID
	ComponentID          uuid.UUID
	VersionControlSystem string
	DefaultRef           *string
}

func (input NewRepositoryComponentMetadata) RepositoryComponentMetadata(now time.Time) (RepositoryComponentMetadata, error) {
	metadata := RepositoryComponentMetadata{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		ComponentID: input.ComponentID, VersionControlSystem: input.VersionControlSystem,
		DefaultRef: input.DefaultRef, CreatedAt: now.UTC(),
	}
	if err := metadata.Validate(); err != nil {
		return RepositoryComponentMetadata{}, err
	}
	return metadata, nil
}

func (metadata RepositoryComponentMetadata) Validate() error {
	if metadata.TenantID == uuid.Nil || metadata.WorkspaceID == uuid.Nil || metadata.ComponentID == uuid.Nil {
		return errors.New("tenant, workspace, and component IDs are required")
	}
	if metadata.TenantID.Version() != 7 || metadata.WorkspaceID.Version() != 7 || metadata.ComponentID.Version() != 7 {
		return errors.New("tenant, workspace, and component IDs must be UUIDv7")
	}
	if strings.TrimSpace(metadata.VersionControlSystem) != metadata.VersionControlSystem ||
		utf8.RuneCountInString(metadata.VersionControlSystem) > maxVersionControlSystemLength ||
		!versionControlSystemPattern.MatchString(metadata.VersionControlSystem) {
		return errors.New("repository version control system is invalid")
	}
	if metadata.DefaultRef != nil && !validRepositoryDefaultRef(*metadata.DefaultRef) {
		return errors.New("repository default ref is invalid")
	}
	if metadata.CreatedAt.IsZero() {
		return errors.New("repository component metadata creation time is required")
	}
	return nil
}

func validRepositoryDefaultRef(ref string) bool {
	if strings.TrimSpace(ref) != ref || ref == "" || utf8.RuneCountInString(ref) > maxRepositoryDefaultRefLength {
		return false
	}
	for _, character := range ref {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
