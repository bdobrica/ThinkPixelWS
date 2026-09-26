package ports

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
)

// MaterializationContentPreparer prepares detached, disposable working storage.
// It must exclusively hold the volume across validate, restore/layout, and
// complete, in that order. validate rejects stale work after acquiring exclusivity;
// complete persists readiness before releasing it. Neither callback grants
// execution authority. Never invoke complete on partial or unverified content.
//
// A false result means asynchronous preparation is pending. The implementation
// may start a consumer for WaitForFirstConsumer volumes before they are Bound.
// Errors leave storage available for retry/authorized cleanup; preparation must
// be idempotent and must not overwrite content of an active Materialization.
type MaterializationContentPreparer interface {
	Prepare(ctx context.Context, m domain.Materialization, validate, complete func(context.Context) error) (bool, error)
}
