package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/reconcile-kit/insight/internal/auth"
	"github.com/reconcile-kit/insight/internal/query"
	"github.com/reconcile-kit/insight/internal/store"
)

type fakeDB struct{ got query.Search }

func (f *fakeDB) List(_ context.Context, s query.Search) (*store.ListResult, error) {
	f.got = s
	n := int64(0)
	return &store.ListResult{Items: []store.Resource{}, Total: &n}, nil
}
func (f *fakeDB) Values(context.Context, string, string, query.Search, int) ([]store.Value, error) {
	return []store.Value{{Value: "Job", Group: "g"}}, nil
}
func (f *fakeDB) Ping(context.Context) error { return nil }

func TestRouting(t *testing.T) {
	var proxied []string
	sm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = append(proxied, r.Method+" "+r.URL.Path)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer sm.Close()
	u, _ := url.Parse(sm.URL)
	db := &fakeDB{}
	h := New(Config{
		StateManagerURL: u,
		Static:          fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}},
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, db)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	if rec := do("POST", "/insight/v1/resources/search", `{"filters":{"kind":"Job"},"limit":5}`); rec.Code != 200 || db.got.Filters.Kind != "Job" {
		t.Fatalf("search: %d %s", rec.Code, rec.Body)
	}
	if rec := do("POST", "/insight/v1/resources/search", `{"bogus":1}`); rec.Code != 400 {
		t.Fatalf("unknown field: %d", rec.Code)
	}
	// Single GET and writes are proxied to state-manager unchanged.
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if rec := do(m, "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", "{}"); rec.Code != 200 {
			t.Fatalf("%s proxy: %d", m, rec.Code)
		}
	}
	if rec := do("POST", "/api/v1/groups/g/namespaces/n/kinds/k/resources/", "{}"); rec.Code != 200 {
		t.Fatalf("create proxy: %d", rec.Code)
	}
	// The state-manager list endpoint is not reachable through insight.
	if rec := do("GET", "/api/v1/resources", ""); rec.Code != 404 {
		t.Fatalf("list must not be proxied: %d", rec.Code)
	}
	if len(proxied) != 4 || proxied[0] != "GET /api/v1/groups/g/namespaces/n/kinds/k/resources/x" {
		t.Fatalf("proxied %v", proxied)
	}
	if rec := do("POST", "/insight/v1/paths", "{}"); rec.Code != 404 {
		t.Fatalf("removed method must be 404: %d", rec.Code)
	}
	// Without MCP tokens /mcp is a JSON 404, not the SPA.
	if rec := do("POST", "/mcp", "{}"); rec.Code != 404 || !strings.Contains(rec.Body.String(), "MCP is disabled") {
		t.Fatalf("disabled mcp: %d %s", rec.Code, rec.Body)
	}
	if rec := do("GET", "/some/client/route", ""); !strings.Contains(rec.Body.String(), "spa") {
		t.Fatalf("spa fallback: %s", rec.Body)
	}
}

func TestAuthGuard(t *testing.T) {
	type seen struct{ method, path, auth, cookie, user string }
	var proxied []seen
	sm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = append(proxied, seen{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Cookie"), r.Header.Get("X-Forwarded-User")})
		_, _ = io.WriteString(w, `{}`)
	}))
	defer sm.Close()
	smURL, _ := url.Parse(sm.URL)
	pub, _ := url.Parse("https://insight.example.com")
	a, err := auth.New(auth.Config{
		IssuerURL: "https://idp.example.com", ClientID: "insight", PublicURL: pub,
		SessionSecret: []byte(strings.Repeat("k", 32)),
		ReadRoles:     []string{"r"}, WriteRoles: []string{"w"}, AdminRoles: []string{"a"},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := New(Config{
		StateManagerURL:   smURL,
		Static:            fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}, "assets/app.js": {Data: []byte("js")}},
		Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:              a,
		StateManagerToken: "sm-service-token",
	}, &fakeDB{})

	session := func(acc auth.Access) string {
		v, err := a.SessionValue(auth.User{Subject: "s", Email: "u@example.com", Access: acc, Expires: time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	do := func(method, path, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: cookie})
		}
		req.Header.Set("Authorization", "Bearer from-browser")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Anonymous: API answers 401, pages redirect to /login keeping the path.
	if rec := do("POST", "/insight/v1/resources/search", ""); rec.Code != 401 {
		t.Fatalf("anonymous search: %d", rec.Code)
	}
	if rec := do("GET", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", ""); rec.Code != 401 {
		t.Fatalf("anonymous get: %d", rec.Code)
	}
	if rec := do("GET", "/?kind=Job", ""); rec.Code != 302 || rec.Header().Get("Location") != "/login?return_to=%2F%3Fkind%3DJob" {
		t.Fatalf("anonymous page: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// Login page, its assets, probes and /auth/me stay public.
	for _, p := range []string{"/login", "/assets/app.js", "/healthz"} {
		if rec := do("GET", p, ""); rec.Code != 200 {
			t.Fatalf("public %s: %d", p, rec.Code)
		}
	}
	if rec := do("GET", "/auth/me", ""); rec.Code != 401 || !strings.Contains(rec.Body.String(), `"auth":true`) {
		t.Fatalf("me anonymous: %d %s", rec.Code, rec.Body)
	}
	if len(proxied) != 0 {
		t.Fatalf("anonymous requests reached state-manager: %+v", proxied)
	}

	// Access levels: read < write < admin (delete).
	steps := []struct {
		acc          auth.Access
		method, path string
		want         int
	}{
		{auth.AccessNone, "POST", "/insight/v1/resources/search", 403},
		{auth.AccessNone, "GET", "/", 200}, // the SPA explains the missing access
		{auth.AccessRead, "POST", "/insight/v1/resources/search", 200},
		{auth.AccessRead, "POST", "/insight/v1/values", 200},
		{auth.AccessRead, "GET", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", 200},
		{auth.AccessRead, "PUT", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", 403},
		{auth.AccessRead, "POST", "/api/v1/groups/g/namespaces/n/kinds/k/resources/", 403},
		{auth.AccessWrite, "PUT", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", 200},
		{auth.AccessWrite, "DELETE", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", 403},
		{auth.AccessAdmin, "DELETE", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", 200},
	}
	for _, s := range steps {
		if rec := do(s.method, s.path, session(s.acc)); rec.Code != s.want {
			t.Errorf("%s %s %s: %d, want %d (%s)", s.acc, s.method, s.path, rec.Code, s.want, rec.Body)
		}
	}
	if rec := do("GET", "/auth/me", session(auth.AccessWrite)); !strings.Contains(rec.Body.String(), `"access":"write"`) {
		t.Fatalf("me: %s", rec.Body)
	}

	// Proxied calls: the session cookie and the browser's Authorization never
	// reach state-manager; the service token and the user are passed instead.
	if len(proxied) != 3 {
		t.Fatalf("proxied %+v", proxied)
	}
	for _, p := range proxied {
		if p.auth != "Bearer sm-service-token" || p.cookie != "" || p.user != "u@example.com" {
			t.Errorf("proxied headers %+v", p)
		}
	}

	// Cross-site writes are rejected even with a valid session.
	req := httptest.NewRequest("PUT", "/api/v1/groups/g/namespaces/n/kinds/k/resources/x", strings.NewReader("{}"))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: session(auth.AccessAdmin)})
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site write: %d", rec.Code)
	}
}
