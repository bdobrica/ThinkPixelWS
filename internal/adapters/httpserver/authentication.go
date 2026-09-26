package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/bdobrica/ThinkPixelWS/internal/config"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type accessContextKey struct{}
type requestAccess struct {
	identity   security.Identity
	authorizer ports.WorkspaceAuthorizer
}

// IdentityFromContext returns identity established by the HTTP auth boundary.
func IdentityFromContext(ctx context.Context) (security.Identity, bool) {
	access, ok := ctx.Value(accessContextKey{}).(requestAccess)
	return access.identity, ok
}

// AuthorizeWorkspace evaluates the explicit action for the authenticated caller.
// Handlers must resolve the target within that caller's tenant and use the same
// tenant/target for the operation. Authentication alone grants no access.
func AuthorizeWorkspace(ctx context.Context, action ports.WorkspaceAction, workspaceID uuid.UUID) error {
	access, ok := ctx.Value(accessContextKey{}).(requestAccess)
	if !ok {
		return security.ErrForbidden
	}
	return security.AuthorizeWorkspace(ctx, access.authorizer, access.identity, action, workspaceID)
}

func developmentAuth(cfg config.AuthConfig) (*security.DevelopmentAuth, error) {
	tenant, err := shared.ParseUUIDv7(cfg.TenantID)
	if err != nil {
		return nil, errors.New("development auth requires a canonical UUIDv7 tenant")
	}
	info, err := os.Stat(cfg.TokenFile)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("development auth token file must be a readable regular file")
	}
	file, err := os.Open(cfg.TokenFile)
	if err != nil {
		return nil, errors.New("cannot open development auth token file")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("development auth token file must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return nil, errors.New("cannot read development auth token file (maximum 4096 bytes)")
	}
	return security.NewDevelopmentAuth(security.Identity{TenantID: uuid.UUID(tenant), Principal: cfg.Principal}, strings.TrimSpace(string(data)))
}

func authenticateDevelopment(auth *security.DevelopmentAuth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Trust the actual peer only, never Forwarded/X-Forwarded-For. Do not deploy
		// this local-only mode behind a proxy or tunnel that exposes it remotely.
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			WriteProblem(w, r, shared.NewError(shared.CodeUnauthorized, "authentication failed"))
			return
		}
		values := r.Header.Values("Authorization")
		var token string
		if len(values) == 1 {
			scheme, value, ok := strings.Cut(values[0], " ")
			if ok && strings.EqualFold(scheme, "Bearer") {
				token = value
			}
		}
		identity, err := auth.Authenticate(r.Context(), token)
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			WriteProblem(w, r, shared.NewError(shared.CodeUnauthorized, "authentication failed"))
			return
		}
		ctx := context.WithValue(r.Context(), accessContextKey{}, requestAccess{identity: identity, authorizer: auth})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
