package contracttest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	api "github.com/bdobrica/ThinkPixelWS/api/openapi"
	"github.com/google/uuid"
)

// This fixture exercises the published wire contract, not AG authorization or
// storage resolution. Production must supply both before exposing the operation.
type bindingFixture struct {
	api.UnimplementedHandler
	request *api.ResolveWorkspaceBinding
	id      api.UUID
	deny    bool
	calls   int
}

func (h *bindingFixture) ResolveWorkspaceBinding(_ context.Context, req *api.ResolveWorkspaceBinding, p api.ResolveWorkspaceBindingParams) (*api.WorkspaceBindingHeaders, error) {
	h.calls++
	h.request, h.id = req, p.MaterializationID
	if h.deny {
		return nil, errors.New("denied")
	}
	return &api.WorkspaceBindingHeaders{CacheControl: api.NewOptString("no-store"), Response: api.WorkspaceBinding{
		WorkspaceId: req.WorkspaceId, Generation: req.Generation, ComponentAccess: req.ComponentAccess,
		MaterializationId: p.MaterializationID, TargetId: req.TargetId, Audience: "thinkpixelar",
		Handle: "opaque:fixture", MountRoot: "/workspace", ExpiresAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}}, nil
}
func (*bindingFixture) NewError(context.Context, error) *api.ProblemStatusCode {
	return &api.ProblemStatusCode{StatusCode: 403, Response: api.Problem{Title: "Forbidden", Status: 403}}
}

type inMemoryHTTP struct{ handler http.Handler }

func (c inMemoryHTTP) Do(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}
func TestWorkspaceBindingClientServer(t *testing.T) {
	fixture := &bindingFixture{}
	server, err := api.NewServer(fixture)
	if err != nil {
		t.Fatal(err)
	}
	client, err := api.NewClient("http://contract.test", api.WithClient(inMemoryHTTP{server}))
	if err != nil {
		t.Fatal(err)
	}
	id := api.UUID(uuid.Must(uuid.NewV7()))
	req := &api.ResolveWorkspaceBinding{WorkspaceId: api.UUID(uuid.Must(uuid.NewV7())), Generation: 42, TargetId: "homelab", ExecutionGrant: "opaque-ag-reference",
		ComponentAccess: []api.WorkspaceBindingComponentAccess{{ComponentId: api.UUID(uuid.Must(uuid.NewV7())), Mode: api.WorkspaceBindingComponentAccessModeReadOnly}}}
	result, err := client.ResolveWorkspaceBinding(context.Background(), req, api.ResolveWorkspaceBindingParams{MaterializationID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req, fixture.request) || fixture.id != id {
		t.Fatal("request scope changed in transit")
	}
	b := result.Response
	if b.WorkspaceId != req.WorkspaceId || b.Generation != 42 || b.MaterializationId != id || b.TargetId != req.TargetId || !reflect.DeepEqual(b.ComponentAccess, req.ComponentAccess) || b.Audience != "thinkpixelar" || b.MountRoot != "/workspace" || b.Handle != "opaque:fixture" || b.ExpiresAt.IsZero() || result.CacheControl.Value != "no-store" {
		t.Fatalf("binding changed in transit: %+v", result)
	}
	fixture.deny = true
	_, err = client.ResolveWorkspaceBinding(context.Background(), req, api.ResolveWorkspaceBindingParams{MaterializationID: id})
	var problem *api.ProblemStatusCode
	if !errors.As(err, &problem) || problem.StatusCode != 403 {
		t.Fatalf("lost RFC 7807 denial: %v", err)
	}
}
func TestWorkspaceBindingRequestValidation(t *testing.T) {
	fixture := &bindingFixture{}
	server, err := api.NewServer(fixture)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.Must(uuid.NewV7()).String()
	valid := `{"workspaceId":"` + id + `","generation":1,"targetId":"homelab","executionGrant":"grant","componentAccess":[{"componentId":"` + id + `","mode":"read-only"}]}`
	for _, body := range []string{
		strings.Replace(valid, `"generation":1`, `"generation":0`, 1),
		strings.Replace(valid, `"executionGrant":"grant"`, `"executionGrant":""`, 1),
		strings.Replace(valid, `"read-only"`, `"admin"`, 1),
		strings.Replace(valid, `"targetId":"homelab"`, `"targetId":""`, 1),
		strings.Replace(valid, `"workspaceId"`, `"tenantId"`, 1),
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/materializations/"+id+"/binding", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		server.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("invalid request accepted: %d %s", rec.Code, rec.Body.String())
		}
	}
	if fixture.calls != 0 {
		t.Fatal("invalid requests reached resolver")
	}
}
