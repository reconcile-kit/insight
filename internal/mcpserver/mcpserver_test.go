package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/reconcile-kit/insight/internal/query"
	"github.com/reconcile-kit/insight/internal/statemanager"
	"github.com/reconcile-kit/insight/internal/store"
)

const (
	readToken  = "read-token-0123456789"
	writeToken = "write-token-0123456789"
)

type fakeDB struct{ got query.Search }

func (f *fakeDB) List(_ context.Context, s query.Search) (*store.ListResult, error) {
	f.got = s
	n := int64(1)
	return &store.ListResult{Items: []store.Resource{{Name: "job-1", Kind: "Job"}}, Total: &n}, nil
}

func (f *fakeDB) Values(_ context.Context, field, _ string, s query.Search, _ int) ([]store.Value, error) {
	f.got = s
	return []store.Value{{Value: field + "-value"}}, nil
}

type fakeSM struct{ calls []string }

func (f *fakeSM) Get(_ context.Context, id statemanager.ID) (json.RawMessage, error) {
	f.calls = append(f.calls, "get "+id.Name)
	return json.RawMessage(`{"name":"` + id.Name + `","spec":{"a":1}}`), nil
}

func (f *fakeSM) Create(_ context.Context, id statemanager.ID, _ any) (json.RawMessage, error) {
	f.calls = append(f.calls, "create "+id.Kind)
	return json.RawMessage(`{"ok":1}`), nil
}

func (f *fakeSM) Update(_ context.Context, id statemanager.ID, _ any) (json.RawMessage, error) {
	f.calls = append(f.calls, "update "+id.Name)
	return json.RawMessage(`{"ok":1}`), nil
}

func (f *fakeSM) Delete(_ context.Context, id statemanager.ID) error {
	f.calls = append(f.calls, "delete "+id.Name)
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeDB, *fakeSM) {
	t.Helper()
	db, sm := &fakeDB{}, &fakeSM{}
	h := Handler(Config{
		ReadTokens:  []string{readToken},
		WriteTokens: []string{writeToken},
		Version:     "test",
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, db, sm)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, db, sm
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: bearerTransport{token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func toolNames(t *testing.T, s *mcp.ClientSession) []string {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	return names
}

func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(r *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestAuth(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	for _, tc := range []struct {
		name, header string
	}{
		{"no header", ""},
		{"wrong token", "Bearer nope-nope-nope-nope"},
		{"basic auth", "Basic " + readToken},
		{"token prefix", "Bearer " + readToken[:10]},
	} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized || res.Header.Get("WWW-Authenticate") == "" {
			t.Errorf("%s: got %d %q, want 401 with WWW-Authenticate", tc.name, res.StatusCode, res.Header.Get("WWW-Authenticate"))
		}
	}
}

func TestReadTokenHasNoWriteTools(t *testing.T) {
	srv, _, sm := newTestServer(t)
	s := connect(t, srv.URL, readToken)

	want := []string{"get_resource", "list_values", "search_resources"}
	if got := toolNames(t, s); !slices.Equal(got, want) {
		t.Fatalf("read tools = %v, want %v", got, want)
	}
	if !strings.Contains(s.InitializeResult().Instructions, "search_resources") ||
		strings.Contains(s.InitializeResult().Instructions, "update_resource") {
		t.Errorf("read instructions mention write tools: %q", s.InitializeResult().Instructions)
	}

	// Calling a write tool by name must fail and never reach state-manager.
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_resource", Arguments: map[string]any{
		"resource_group": "g", "namespace": "ns", "kind": "Job", "name": "x",
	}})
	if err == nil && !res.IsError {
		t.Fatalf("delete_resource with read token succeeded: %s", text(res))
	}
	if len(sm.calls) != 0 {
		t.Fatalf("state-manager called with read token: %v", sm.calls)
	}
}

func TestWriteToken(t *testing.T) {
	srv, _, sm := newTestServer(t)
	s := connect(t, srv.URL, writeToken)

	want := []string{"create_resource", "delete_resource", "get_resource", "list_values", "search_resources", "update_resource"}
	if got := toolNames(t, s); !slices.Equal(got, want) {
		t.Fatalf("write tools = %v, want %v", got, want)
	}

	id := map[string]any{"resource_group": "g", "namespace": "ns", "kind": "Job", "name": "x"}
	with := func(extra map[string]any) map[string]any {
		m := map[string]any{}
		for k, v := range id {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if r := call(t, s, "create_resource", with(map[string]any{"shard_id": "s1", "spec": map[string]any{"a": 1}})); r.IsError {
		t.Fatalf("create: %s", text(r))
	}
	if r := call(t, s, "update_resource", with(map[string]any{"shard_id": "s1", "spec": map[string]any{"a": 2}, "version": 3})); r.IsError {
		t.Fatalf("update: %s", text(r))
	}
	// update without spec would wipe it: rejected before state-manager.
	if r := call(t, s, "update_resource", with(map[string]any{"shard_id": "s1"})); !r.IsError {
		t.Fatalf("update without spec succeeded")
	}
	if r := call(t, s, "delete_resource", id); r.IsError {
		t.Fatalf("delete: %s", text(r))
	}
	if want := []string{"create Job", "update x", "delete x"}; !slices.Equal(sm.calls, want) {
		t.Fatalf("state-manager calls = %v, want %v", sm.calls, want)
	}
}

func TestReadToolsRouting(t *testing.T) {
	srv, db, sm := newTestServer(t)
	s := connect(t, srv.URL, readToken)

	r := call(t, s, "search_resources", map[string]any{
		"filters": map[string]any{"kind": "Job"},
		"labels":  "app=web",
		"json":    []any{map[string]any{"root": "status", "path": "exit_code", "op": "ne", "value": "0"}},
		"sort":    map[string]any{"field": "updated_at", "desc": true},
	})
	if r.IsError || !strings.Contains(text(r), "job-1") {
		t.Fatalf("search: %s", text(r))
	}
	g := db.got
	if g.Filters.Kind != "Job" || g.Labels != "app=web" || len(g.JSON) != 1 || g.JSON[0].Op != "ne" ||
		g.Sort.Field != "updated_at" || !g.Sort.Desc {
		t.Fatalf("search request = %+v", g)
	}

	// A half-filled JSON condition must not silently widen the result.
	r = call(t, s, "search_resources", map[string]any{
		"json": []any{map[string]any{"root": "status", "path": "phase", "op": "eq"}},
	})
	if !r.IsError {
		t.Fatalf("condition without value accepted: %s", text(r))
	}

	r = call(t, s, "list_values", map[string]any{"field": "kind", "search": map[string]any{"filters": map[string]any{"namespace": "prod"}}})
	if r.IsError || !strings.Contains(text(r), "kind-value") || db.got.Filters.Namespace != "prod" {
		t.Fatalf("values: %s / %+v", text(r), db.got)
	}

	r = call(t, s, "get_resource", map[string]any{"resource_group": "g", "namespace": "ns", "kind": "Job", "name": "x"})
	if r.IsError || !strings.Contains(text(r), `"spec"`) {
		t.Fatalf("get: %s", text(r))
	}
	if !slices.Equal(sm.calls, []string{"get x"}) {
		t.Fatalf("state-manager calls = %v", sm.calls)
	}
}
