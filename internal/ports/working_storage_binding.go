package ports

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/api/storagebinding"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
)

// WorkingStorageBinding is a storage description, never an authorization grant.
// Kind versions the provider-specific Reference consumed by a trusted runtime
// adapter. Reference must contain no credentials. Handle pins storage identity.
type WorkingStorageBinding = storagebinding.Binding

// WorkingStorageBindingProvider resolves existing, prepared storage without
// attaching execution or changing resources. Call only with authorized records.
type WorkingStorageBindingProvider interface {
	Binding(context.Context, domain.Materialization) (WorkingStorageBinding, error)
}
