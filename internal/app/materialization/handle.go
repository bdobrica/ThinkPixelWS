package materialization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

const bindingAudience = "thinkpixelar"

// bindingHandle deliberately contains neither provider instructions nor secrets.
// The digest binds a reference to a particular observed record, not to authority:
// it is NOT a signature or MAC. Consumers keep the entire value opaque.
func bindingHandle(m domain.Materialization) string {
	// JSON array encoding avoids ambiguous concatenation of variable-length fields.
	identity, _ := json.Marshal([]any{bindingAudience, m.TenantID, m.ID,
		m.WorkspaceID, m.BaseGeneration, m.StateVersion, m.Target.ID, m.Mode,
		m.Provider, m.Handle})
	digest := sha256.Sum256(identity)
	return "ws-mat-v1:" + m.ID.String() + ":" + hex.EncodeToString(digest[:])
}

// ResolveHandle resolves a previously returned opaque WS handle after the caller
// independently authorizes this tenant, Materialization and exact component scope.
// The ID must be the independently authorized Materialization, not derived from
// this untrusted handle. Target and audience come from trusted runtime configuration.
// This method does not validate a grant, its expiry, or a writer lease; those checks remain
// mandatory at attachment/use. It neither allocates nor attaches storage.
// State-version changes invalidate old handles, including READY -> ACTIVE.
func (r BindingReader) ResolveHandle(ctx context.Context, tenant, id uuid.UUID, target, audience, handle string) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	parts := strings.Split(handle, ":")
	if audience != bindingAudience || target == "" || len(parts) != 3 || parts[0] != "ws-mat-v1" || len(handle) != len("ws-mat-v1:")+36+1+sha256.Size*2 {
		return Binding{}, ports.ErrMaterializationStateConflict
	}
	handleID, err := uuid.Parse(parts[1])
	if err != nil || handleID != id || id.Version() != 7 || id.Variant() != uuid.RFC4122 {
		return Binding{}, ports.ErrMaterializationStateConflict
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return Binding{}, ports.ErrMaterializationStateConflict
	}
	binding, err := r.Binding(ctx, tenant, id)
	if err != nil {
		return Binding{}, err
	}
	if binding.TargetID != target || binding.Handle != handle {
		return Binding{}, ports.ErrMaterializationStateConflict
	}
	return binding, nil
}
