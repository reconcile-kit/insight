package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/reconcile-kit/insight/internal/auth"
)

// requiredAccess maps a request to the access level it needs. The split
// matches the read/edit/admin roles of an auth proxy: reads (including the
// POST search methods), create/update, delete.
func requiredAccess(r *http.Request) (auth.Access, bool) {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/insight/"):
		return auth.AccessRead, true
	case strings.HasPrefix(p, "/api/"):
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return auth.AccessRead, true
		case http.MethodDelete:
			return auth.AccessAdmin, true
		default:
			return auth.AccessWrite, true
		}
	}
	// The SPA itself only needs a session: without access it explains why.
	return auth.AccessNone, false
}

// isPublic lists what is reachable without a session: probes, the auth flow,
// MCP (own tokens), and the login page with its assets.
func isPublic(p string) bool {
	switch {
	case p == "/healthz", p == "/readyz", p == "/mcp", p == "/login", p == "/favicon.svg":
		return true
	case strings.HasPrefix(p, "/auth/"), strings.HasPrefix(p, "/assets/"):
		return true
	}
	return false
}

func isAPI(p string) bool { return strings.HasPrefix(p, "/insight/") || strings.HasPrefix(p, "/api/") }

type userKey struct{}

func userFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userKey{}).(*auth.User)
	return u
}

// guard requires a session for everything but public paths and checks the
// access level of API calls. Pages without a session redirect to /login.
func guard(a *auth.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			u, ok := a.UserFrom(r)
			if !ok {
				if isAPI(r.URL.Path) {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required"})
					return
				}
				http.Redirect(w, r, "/login?return_to="+url.QueryEscape(auth.SafeReturnTo(r.URL.RequestURI())), http.StatusFound)
				return
			}
			setLogUser(r.Context(), u.Email+" ("+u.Access.String()+")")
			if need, api := requiredAccess(r); api && u.Access < need {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "access denied: " + need.String() + " access required, you have " + u.Access.String(),
				})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
		})
	}
}

type meResponse struct {
	// Auth is false when insight runs without login (everyone has full access).
	Auth   bool        `json:"auth"`
	User   *auth.User  `json:"user,omitempty"`
	Access auth.Access `json:"access"`
}

// me tells the SPA who is logged in and what they may do: GET /auth/me.
func me(a *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if a == nil {
			writeJSON(w, http.StatusOK, meResponse{Auth: false, Access: auth.AccessAdmin})
			return
		}
		u, ok := a.UserFrom(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, meResponse{Auth: true})
			return
		}
		writeJSON(w, http.StatusOK, meResponse{Auth: true, User: u, Access: u.Access})
	}
}

// logUser lets guard (inside the router) report the user to requestLogger (outside it).
type logUser struct{ v string }

type logUserKey struct{}

func setLogUser(ctx context.Context, v string) {
	if h, ok := ctx.Value(logUserKey{}).(*logUser); ok {
		h.v = v
	}
}
