// Package layout prepares component directories on mounted working storage.
package layout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

// Component maps a stable component identity to its volume-relative directory
// and canonical execution path. The volume root is mounted at /workspace.
type Component struct {
	ID            uuid.UUID
	RelativePath  string
	CanonicalPath string
}

// Prepare creates the selected generation's component directories in name order.
// root must be an already-open, trusted mounted volume root, exclusively held by
// the preparer with execution detached. Callers authorize the operation and load
// the complete, tenant-scoped generation membership before calling. This helper
// neither proves membership nor grants authority or marks a Materialization READY.
// It works with local-path and CSI filesystem volumes alike.
//
// Existing directories and contents are preserved; files and symlinks conflict.
// All records and existing paths are checked before creating directories. I/O
// failure or cancellation can leave a partial layout; retry is safe. No cleanup,
// chmod, content restore, mount, or Kubernetes API operation is performed.
func Prepare(ctx context.Context, root *os.Root, tenantID, workspaceID uuid.UUID, components []domain.WorkspaceComponent) ([]Component, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == nil || tenantID == uuid.Nil || tenantID.Version() != 7 || workspaceID == uuid.Nil || workspaceID.Version() != 7 {
		return nil, errors.New("layout requires a volume root and UUIDv7 tenant and workspace IDs")
	}
	result := make([]Component, 0, len(components))
	names := make(map[string]bool, len(components))
	ids := make(map[uuid.UUID]bool, len(components))
	for _, component := range components {
		if err := component.Validate(); err != nil {
			return nil, fmt.Errorf("invalid layout component: %w", err)
		}
		if component.TenantID != tenantID || component.WorkspaceID != workspaceID {
			return nil, errors.New("layout component scope mismatch")
		}
		if names[component.Name] || ids[component.ID] {
			return nil, errors.New("duplicate layout component name or ID")
		}
		names[component.Name], ids[component.ID] = true, true
		result = append(result, Component{ID: component.ID, RelativePath: component.Name, CanonicalPath: component.CanonicalPath})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RelativePath < result[j].RelativePath })
	for _, component := range result {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := checkDirectory(root, component.RelativePath); err != nil {
			return nil, err
		}
	}
	for _, component := range result {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := root.Mkdir(component.RelativePath, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create component directory: %w", err)
		}
		// Lstat rejects symlinks, including links to another component directory.
		info, err := root.Lstat(component.RelativePath)
		if err != nil {
			return nil, fmt.Errorf("inspect component directory: %w", err)
		}
		if !info.IsDir() {
			return nil, errors.New("component path is not a directory")
		}
	}
	return result, nil
}

func checkDirectory(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect component directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("component path is not a directory")
	}
	return nil
}
