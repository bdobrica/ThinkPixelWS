package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewDirectoryComponentMetadata(t *testing.T) {
	t.Parallel()

	input := validNewDirectoryComponentMetadata(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	metadata, err := input.DirectoryComponentMetadata(now)
	if err != nil {
		t.Fatalf("create directory component metadata: %v", err)
	}
	if metadata.TenantID != input.TenantID || metadata.WorkspaceID != input.WorkspaceID ||
		metadata.ComponentID != input.ComponentID || metadata.CreatedAt.Location() != time.UTC ||
		!metadata.CreatedAt.Equal(now) {
		t.Fatalf("unexpected directory component metadata: %#v", metadata)
	}
}

func TestNewDocumentCollectionComponentMetadata(t *testing.T) {
	t.Parallel()

	input := validNewDocumentCollectionComponentMetadata(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	metadata, err := input.DocumentCollectionComponentMetadata(now)
	if err != nil {
		t.Fatalf("create document collection component metadata: %v", err)
	}
	if metadata.TenantID != input.TenantID || metadata.WorkspaceID != input.WorkspaceID ||
		metadata.ComponentID != input.ComponentID || metadata.CreatedAt.Location() != time.UTC ||
		!metadata.CreatedAt.Equal(now) {
		t.Fatalf("unexpected document collection component metadata: %#v", metadata)
	}
}

func TestDirectoryComponentMetadataRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewDirectoryComponentMetadata){
		"non-v7 tenant ID":    func(input *NewDirectoryComponentMetadata) { input.TenantID = uuid.New() },
		"non-v7 workspace ID": func(input *NewDirectoryComponentMetadata) { input.WorkspaceID = uuid.New() },
		"non-v7 component ID": func(input *NewDirectoryComponentMetadata) { input.ComponentID = uuid.New() },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewDirectoryComponentMetadata(t)
			mutate(&input)
			if _, err := input.DirectoryComponentMetadata(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if _, err := validNewDirectoryComponentMetadata(t).DirectoryComponentMetadata(time.Time{}); err == nil {
		t.Fatal("expected zero creation time validation error")
	}
}

func TestDocumentCollectionComponentMetadataRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewDocumentCollectionComponentMetadata){
		"non-v7 tenant ID":    func(input *NewDocumentCollectionComponentMetadata) { input.TenantID = uuid.New() },
		"non-v7 workspace ID": func(input *NewDocumentCollectionComponentMetadata) { input.WorkspaceID = uuid.New() },
		"non-v7 component ID": func(input *NewDocumentCollectionComponentMetadata) { input.ComponentID = uuid.New() },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewDocumentCollectionComponentMetadata(t)
			mutate(&input)
			if _, err := input.DocumentCollectionComponentMetadata(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if _, err := validNewDocumentCollectionComponentMetadata(t).DocumentCollectionComponentMetadata(time.Time{}); err == nil {
		t.Fatal("expected zero creation time validation error")
	}
}

func validNewDirectoryComponentMetadata(t *testing.T) NewDirectoryComponentMetadata {
	t.Helper()
	return NewDirectoryComponentMetadata{
		TenantID: newMetadataUUIDv7(t), WorkspaceID: newMetadataUUIDv7(t), ComponentID: newMetadataUUIDv7(t),
	}
}

func validNewDocumentCollectionComponentMetadata(t *testing.T) NewDocumentCollectionComponentMetadata {
	t.Helper()
	return NewDocumentCollectionComponentMetadata{
		TenantID: newMetadataUUIDv7(t), WorkspaceID: newMetadataUUIDv7(t), ComponentID: newMetadataUUIDv7(t),
	}
}

func newMetadataUUIDv7(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
