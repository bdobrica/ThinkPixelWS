package domain

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type SourceBindingMode string

const (
	SourceBindingSnapshot            SourceBindingMode = "snapshot"
	SourceBindingRefreshableSnapshot SourceBindingMode = "refreshable-snapshot"
	SourceBindingLiveReference       SourceBindingMode = "live-reference"

	maxSourceProviderLength = 63
	maxSourceRefLength      = 2048
	maxSourceRevisionLength = 512
)

var sourceProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,62}$`)

// SourceBinding describes how a component is acquired from a credential-free
// external source reference. It conveys no authority to access or modify that
// source; resolved generation provenance is recorded separately.
type SourceBinding struct {
	TenantID             uuid.UUID
	WorkspaceID          uuid.UUID
	ComponentID          uuid.UUID
	Provider             string
	Ref                  string
	Mode                 SourceBindingMode
	LastResolvedRevision *string
	CreatedAt            time.Time
}

type NewSourceBinding struct {
	TenantID             uuid.UUID
	WorkspaceID          uuid.UUID
	ComponentID          uuid.UUID
	Provider             string
	Ref                  string
	Mode                 SourceBindingMode
	LastResolvedRevision *string
}

func (input NewSourceBinding) SourceBinding(now time.Time) (SourceBinding, error) {
	binding := SourceBinding{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		ComponentID: input.ComponentID, Provider: input.Provider, Ref: input.Ref,
		Mode: input.Mode, LastResolvedRevision: input.LastResolvedRevision,
		CreatedAt: now.UTC(),
	}
	if err := binding.Validate(); err != nil {
		return SourceBinding{}, err
	}
	return binding, nil
}

func (binding SourceBinding) Validate() error {
	if binding.TenantID == uuid.Nil || binding.WorkspaceID == uuid.Nil || binding.ComponentID == uuid.Nil {
		return errors.New("source binding tenant, workspace, and component IDs are required")
	}
	if binding.TenantID.Version() != 7 || binding.WorkspaceID.Version() != 7 || binding.ComponentID.Version() != 7 {
		return errors.New("source binding tenant, workspace, and component IDs must be UUIDv7")
	}
	if utf8.RuneCountInString(binding.Provider) > maxSourceProviderLength || !sourceProviderPattern.MatchString(binding.Provider) {
		return errors.New("source binding provider is invalid")
	}
	if !validSourceRef(binding.Ref) {
		return errors.New("source binding ref is invalid")
	}
	if !binding.Mode.valid() {
		return errors.New("source binding mode is invalid")
	}
	if binding.LastResolvedRevision != nil && !validBoundedSourceValue(*binding.LastResolvedRevision, maxSourceRevisionLength) {
		return errors.New("source binding last resolved revision is invalid")
	}
	if binding.CreatedAt.IsZero() {
		return errors.New("source binding creation time is required")
	}
	return nil
}

func (mode SourceBindingMode) valid() bool {
	switch mode {
	case SourceBindingSnapshot, SourceBindingRefreshableSnapshot, SourceBindingLiveReference:
		return true
	default:
		return false
	}
}

func validSourceRef(ref string) bool {
	if !validBoundedSourceValue(ref, maxSourceRefLength) {
		return false
	}
	parsed, err := url.Parse(ref)
	return err == nil && parsed.IsAbs() && parsed.Scheme != "" && parsed.User == nil
}

func validBoundedSourceValue(value string, maximum int) bool {
	if strings.TrimSpace(value) != value || value == "" || utf8.RuneCountInString(value) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
