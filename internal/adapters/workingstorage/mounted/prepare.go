// Package mounted prepares generation content on exclusively mounted hot storage.
package mounted

import (
	"context"
	"errors"
	"os"

	"github.com/bdobrica/ThinkPixelWS/internal/adapters/workingstorage/layout"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
)

type Preparer struct {
	// WithRoot obtains exclusive access to the exact bound volume, with execution
	// detached, and holds it until use returns. It owns opening/closing the root.
	// It may start a mount consumer and return false while scheduling is pending.
	// A synchronous use callback must be called exactly once when mounted.
	WithRoot func(ctx context.Context, m domain.Materialization, use func(*os.Root) error) (bool, error)
	// Restore restores and verifies the exact tenant/Workspace/base generation,
	// including its manifest, and returns its complete component membership.
	// It must be confined to root and idempotent after partial failure. No source
	// credentials or user-controlled host paths are accepted by this adapter.
	Restore func(context.Context, domain.Materialization, *os.Root) ([]domain.WorkspaceComponent, error)
}

var _ ports.MaterializationContentPreparer = Preparer{}

func (p Preparer) Prepare(ctx context.Context, m domain.Materialization, validate, complete func(context.Context) error) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if p.WithRoot == nil || p.Restore == nil || validate == nil || complete == nil {
		return false, errors.New("mounted preparation is not configured")
	}
	if err := m.Validate(); err != nil {
		return false, err
	}
	if m.State != domain.MaterializationPreparing || m.Handle == "" {
		return false, ports.ErrMaterializationStateConflict
	}
	called, completed := false, false
	ready, err := p.WithRoot(ctx, m, func(root *os.Root) error {
		if called || root == nil {
			return errors.New("invalid preparation mount")
		}
		called = true
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validate(ctx); err != nil {
			return err
		}
		components, err := p.Restore(ctx, m, root)
		if err != nil {
			return err
		}
		if _, err := layout.Prepare(ctx, root, m.TenantID, m.WorkspaceID, components); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := complete(ctx); err != nil {
			return err
		}
		completed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if ready != completed || called != completed {
		return false, errors.New("inconsistent preparation mount result")
	}
	return completed, nil
}
