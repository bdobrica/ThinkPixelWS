package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/google/uuid"
)

func TestWorkspaceBindingUnavailableUntilAuthorizedResolverIsWired(t *testing.T) {
	handler, err := NewWorkspaceAPI(workspace.Creator{}, workspace.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/materializations/"+uuid.Must(uuid.NewV7()).String()+"/binding", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 404 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("unexpected binding exposure: %d %s", rec.Code, rec.Body.String())
	}
}
