package materialization

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type renewalMaterializations struct {
	ports.MaterializationRepository
	m domain.Materialization
}

func (r renewalMaterializations) Get(context.Context, uuid.UUID, uuid.UUID) (domain.Materialization, error) {
	return r.m, nil
}

type renewalLeases struct {
	ports.MaterializationLeaseRepository
	calls  int
	expiry time.Time
}

func (r *renewalLeases) Renew(_ context.Context, tenant, m, lease uuid.UUID, fence uint64, holder string, expiry time.Time) (domain.MaterializationLease, error) {
	r.calls++
	r.expiry = expiry
	return domain.MaterializationLease{TenantID: tenant, MaterializationID: m, ID: lease, FencingToken: fence, Holder: holder}, nil
}
func TestLeaseRenewalAuthority(t *testing.T) {
	for _, loss := range []string{"expiry", "revocation", "wrong Run", "wrong Execution", "read-only", "wrong workspace", "missing component"} {
		t.Run(loss, func(t *testing.T) {
			id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
			now := time.Now().UTC()
			m := domain.Materialization{TenantID: id(), ID: id(), WorkspaceID: id(), RunID: id(), ExecutionID: id(), BaseGeneration: 1, Mode: domain.MaterializationReadWrite}
			access := []ports.ExecutionComponentAccess{{ComponentID: id(), Mode: domain.MaterializationReadWrite}}
			a := ports.ExecutionAuthority{Issuer: "ag", Audience: ports.ExecutionAuthorityAudience, GrantID: "grant", TenantID: m.TenantID, Principal: "alice", RunID: m.RunID, ExecutionID: m.ExecutionID, WorkspaceID: m.WorkspaceID, ComponentAccess: access, IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(30 * time.Second)}
			calls := 0
			var failure error
			leases := &renewalLeases{}
			r := LeaseRenewer{Materializations: renewalMaterializations{m: m}, Leases: leases, Scope: RequestScopeVerifier{Clock: requestClockFunc(func() time.Time { return now }), Verifier: requestAuthorityVerifier(func(context.Context, ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
				calls++
				return a, failure
			})}}
			lease := id()
			renew := func() (domain.MaterializationLease, error) {
				return r.Renew(t.Context(), m.TenantID, m.ID, lease, 7, "holder", "private-grant", access)
			}
			got, err := renew()
			if err != nil || got.ID != lease || got.MaterializationID != m.ID || got.FencingToken != 7 || got.Holder != "holder" || !leases.expiry.Equal(a.ExpiresAt) {
				t.Fatalf("renewal: %+v %v", got, err)
			}
			switch loss {
			case "expiry":
				now = a.ExpiresAt
			case "revocation":
				failure = errors.New("revoked private-grant")
			case "wrong Run":
				a.RunID = id()
			case "wrong Execution":
				a.ExecutionID = uuid.Nil
			case "read-only":
				a.ComponentAccess = []ports.ExecutionComponentAccess{{ComponentID: access[0].ComponentID, Mode: domain.MaterializationReadOnly}}
			case "wrong workspace":
				a.WorkspaceID = id()
			case "missing component":
				a.ComponentAccess = nil
			}
			got, err = renew()
			if err != security.ErrExecutionAuthority || got != (domain.MaterializationLease{}) || leases.calls != 1 || calls != 2 {
				t.Fatalf("authority loss renewed or leaked: %+v %v calls=%d/%d", got, err, leases.calls, calls)
			}
		})
	}
}
