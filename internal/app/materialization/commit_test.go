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

type commitRecorder struct {
	calls int
	input ports.GenerationCommit
}

func (r *commitRecorder) Commit(_ context.Context, in ports.GenerationCommit) (domain.WorkspaceGeneration, error) {
	r.calls++
	r.input = in
	return domain.WorkspaceGeneration{ID: in.GenerationID}, nil
}
func TestGenerationCommitAuthority(t *testing.T) {
	for _, loss := range []string{"expiry", "revocation", "wrong Run", "wrong Execution", "read-only", "wrong workspace", "missing component"} {
		t.Run(loss, func(t *testing.T) {
			id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
			now := time.Now().UTC()
			m := domain.Materialization{TenantID: id(), ID: id(), WorkspaceID: id(), RunID: id(), ExecutionID: id(), BaseGeneration: 1, Mode: domain.MaterializationReadWrite}
			access := []ports.ExecutionComponentAccess{{ComponentID: id(), Mode: domain.MaterializationReadWrite}}
			a := ports.ExecutionAuthority{Issuer: "ag", Audience: ports.ExecutionAuthorityAudience, GrantID: "grant", TenantID: m.TenantID, Principal: "alice", RunID: m.RunID, ExecutionID: m.ExecutionID, WorkspaceID: m.WorkspaceID, ComponentAccess: access, IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(30 * time.Second)}
			calls := 0
			var failure error
			commits := &commitRecorder{}
			r := GenerationCommitter{Materializations: renewalMaterializations{m: m}, Generations: commits, Scope: RequestScopeVerifier{Clock: requestClockFunc(func() time.Time { return now }), Verifier: requestAuthorityVerifier(func(context.Context, ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
				calls++
				return a, failure
			})}}

			in := ports.GenerationCommit{Writer: ports.MaterializationWriter{TenantID: m.TenantID, WorkspaceID: m.WorkspaceID, MaterializationID: m.ID}, GenerationID: id(), ComponentReferences: []domain.GenerationComponentReference{{ComponentID: access[0].ComponentID}}, Principal: "untrusted", AuthorityExpiresAt: now.Add(time.Hour)}
			commit := func() (domain.WorkspaceGeneration, error) { return r.Commit(t.Context(), in, "private-grant") }
			got, err := commit()
			if err != nil || got.ID != in.GenerationID || !commits.input.AuthorityExpiresAt.Equal(a.ExpiresAt) || commits.input.Principal != a.Principal || commits.input.RunID == nil || *commits.input.RunID != a.RunID || commits.input.ExecutionID == nil || *commits.input.ExecutionID != a.ExecutionID {
				t.Fatalf("commit attribution/deadline: %+v %v", commits.input, err)
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
			got, err = commit()
			if err != security.ErrExecutionAuthority || got.ID != uuid.Nil || commits.calls != 1 || calls != 2 {
				t.Fatalf("authority loss committed or leaked: %+v %v calls=%d/%d", got, err, commits.calls, calls)
			}
		})
	}
}
