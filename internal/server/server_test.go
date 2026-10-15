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
func (f *fakeDB) Paths(context.Context, string, query.Search, int) ([]store.Path, error) {
	return nil, nil
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
	if rec := do("GET", "/some/client/route", ""); !strings.Contains(rec.Body.String(), "spa") {
		t.Fatalf("spa fallback: %s", rec.Body)
	}
}
