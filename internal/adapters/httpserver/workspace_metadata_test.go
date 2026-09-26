package httpserver

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type metadataParent struct {
	ports.WorkspaceReader
	err error
}

func (p metadataParent) GetWorkspace(_ context.Context, tenant, id uuid.UUID) (ports.WorkspaceRecord, error) {
	return ports.WorkspaceRecord{Workspace: domain.Workspace{TenantID: tenant, ID: id}}, p.err
}

type failingMetadata struct{ calls, limit int }

func (m *failingMetadata) GetComponent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (ports.ComponentRecord, error) {
	m.calls++
	return ports.ComponentRecord{}, errors.New("secret-database-error")
}

func (m *failingMetadata) GetGeneration(context.Context, uuid.UUID, uuid.UUID, int) (domain.WorkspaceGeneration, error) {
	m.calls++
	return domain.WorkspaceGeneration{}, errors.New("secret-database-error")
}

func (m *failingMetadata) ListComponents(_ context.Context, _, _, _ uuid.UUID, limit int) ([]ports.ComponentRecord, error) {
	m.calls++
	m.limit = limit
	return nil, errors.New("secret-database-error")
}

func (m *failingMetadata) ListGenerations(_ context.Context, _, _ uuid.UUID, after, limit int) ([]domain.WorkspaceGeneration, error) {
	m.calls++
	m.limit = limit
	return nil, errors.New("secret-database-error")
}

func TestWorkspaceMetadataHTTPBoundaries(t *testing.T) {
	tenant, id, component := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	codec, _ := security.NewCursorCodec(bytes.Repeat([]byte{3}, 32), clockadapter.System{})
	store := &failingMetadata{}
	for _, suffix := range []string{"/components", "/generations", "/components/" + component.String(), "/generations/1"} {
		for _, test := range []struct {
			name         string
			parentErr    error
			policy       ports.WorkspaceAuthorizer
			missingStore bool
			status       int
		}{
			{name: "missing parent", parentErr: ports.ErrWorkspaceNotFound, policy: readPolicy{}, status: 404},
			{name: "parent failure", parentErr: errors.New("secret-database-error"), policy: readPolicy{}, status: 503},
			{name: "no policy", status: 403},
			{name: "policy deny", policy: readPolicy{hidden: id}, status: 403},
			{name: "metadata failure", policy: readPolicy{}, status: 503},
			{name: "missing metadata adapter", policy: readPolicy{}, missingStore: true, status: 503},
		} {
			t.Run(suffix+"/"+test.name, func(t *testing.T) {
				reader := workspace.Reader{Store: metadataParent{err: test.parentErr}, Metadata: store, Cursors: codec, Clock: clockadapter.System{}}
				if test.missingStore {
					reader.Metadata = nil
				}
				handler, err := NewWorkspaceAPI(workspace.Creator{}, reader)
				if err != nil {
					t.Fatal(err)
				}
				before := store.calls
				req := httptest.NewRequest("GET", "/v1/workspaces/"+id.String()+suffix, nil)
				req = req.WithContext(context.WithValue(req.Context(), accessContextKey{}, requestAccess{identity: security.Identity{TenantID: tenant, Principal: "alice"}, authorizer: test.policy}))
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != test.status || strings.Contains(rec.Body.String(), "secret-database-error") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/problem+json") {
					t.Fatalf("response: %d %s", rec.Code, rec.Body.String())
				}
				if test.name == "metadata failure" {
					if store.calls != before+1 {
						t.Fatal("metadata was not read")
					}
					if !strings.Contains(suffix[1:], "/") && store.limit != 101 {
						t.Fatalf("default limit: %d", store.limit)
					}
				} else if store.calls != before {
					t.Fatal("metadata read before parent resolution and authorization")
				}
			})
		}
	}
}
