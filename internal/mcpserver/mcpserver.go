// Package mcpserver exposes insight to AI agents over MCP (streamable HTTP at /mcp).
//
// Tools are built on the same Go implementation as the insight HTTP methods
// and follow the same routing:
//
//   - search_resources  = POST /insight/v1/resources/search -> store.List (database, read-only)
//   - list_values       = POST /insight/v1/values           -> store.Values (database, read-only)
//   - get_resource      = GET one resource                  -> state-manager API
//   - create/update/delete_resource                         -> state-manager API
//
// /mcp has its own authorization, independent of any proxy in front of the UI:
// two static bearer tokens. A read token gets a server with the three read
// tools only, so write tools cannot be called with it even by name; a write
// token gets all six.
package mcpserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/reconcile-kit/insight/internal/query"
	"github.com/reconcile-kit/insight/internal/statemanager"
	"github.com/reconcile-kit/insight/internal/store"
)

// Reader is the read side of the database store (the same one the
// /insight/v1 methods use).
type Reader interface {
	List(ctx context.Context, req query.Search) (*store.ListResult, error)
	Values(ctx context.Context, field, q string, req query.Search, limit int) ([]store.Value, error)
}

// StateManager is the single-resource API of state-manager.
type StateManager interface {
	Get(ctx context.Context, id statemanager.ID) (json.RawMessage, error)
	Create(ctx context.Context, id statemanager.ID, body any) (json.RawMessage, error)
	Update(ctx context.Context, id statemanager.ID, body any) (json.RawMessage, error)
	Delete(ctx context.Context, id statemanager.ID) error
}

type Access string

const (
	AccessRead  Access = "read"
	AccessWrite Access = "write"
)

type Config struct {
	// ReadTokens and WriteTokens are the accepted bearer tokens. Several tokens
	// per level allow rotation without downtime.
	ReadTokens  []string
	WriteTokens []string
	Version     string
	Log         *slog.Logger
}

// Enabled reports whether at least one token is configured.
func (c Config) Enabled() bool { return len(c.ReadTokens)+len(c.WriteTokens) > 0 }

// Handler returns the /mcp handler: bearer token check, then a stateless
// streamable HTTP MCP server for the token's access level.
func Handler(cfg Config, db Reader, sm StateManager) http.Handler {
	t := &tools{db: db, sm: sm, log: cfg.Log}
	servers := map[Access]*mcp.Server{
		AccessRead:  t.server(AccessRead, cfg.Version),
		AccessWrite: t.server(AccessWrite, cfg.Version),
	}
	tokens := newTokenSet(cfg.ReadTokens, cfg.WriteTokens)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return servers[accessFrom(r.Context())]
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		access, ok := tokens.lookup(bearer(r.Header.Get("Authorization")))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="insight-mcp"`)
			http.Error(w, `{"error":"missing or invalid bearer token"}`, http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accessKey{}, access)))
	})
}

type accessKey struct{}

func accessFrom(ctx context.Context) Access {
	a, _ := ctx.Value(accessKey{}).(Access)
	return a
}

func bearer(h string) string {
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// tokenSet compares SHA-256 digests in constant time, so neither the token
// contents nor their lengths leak through timing.
type tokenSet struct {
	entries []tokenEntry
}

type tokenEntry struct {
	sum    [32]byte
	access Access
}

func newTokenSet(read, write []string) *tokenSet {
	s := &tokenSet{}
	for _, t := range write {
		s.entries = append(s.entries, tokenEntry{sha256.Sum256([]byte(t)), AccessWrite})
	}
	for _, t := range read {
		s.entries = append(s.entries, tokenEntry{sha256.Sum256([]byte(t)), AccessRead})
	}
	return s
}

func (s *tokenSet) lookup(token string) (Access, bool) {
	if token == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(token))
	var found Access
	for _, e := range s.entries {
		if subtle.ConstantTimeCompare(sum[:], e.sum[:]) == 1 && found == "" {
			found = e.access
		}
	}
	return found, found != ""
}

const instructions = `Insight is a read-mostly view of the reconcile-kit state-manager: resources identified by
resource_group / namespace / kind / name, each with shard_id, labels, annotations, finalizers,
spec (desired state, written by users) and status (observed state, written by controllers).
version > current_version means the spec change is not reconciled yet (pending).

Use search_resources to find resources (filters, label selector, filters on any JSON field of
spec/status/annotations/finalizers, sorting, paging, total count). Use list_values to discover
existing groups, kinds, namespaces and shards. search_resources returns only metadata; call
get_resource for the full body with spec and status.`

const writeInstructions = `

Writes go through the state-manager API. update_resource replaces spec, labels, annotations and
finalizers as a whole: call get_resource first, change what you need, send everything back with
the version you read to avoid overwriting a concurrent change. delete_resource marks the resource
for deletion; it disappears once controllers remove their finalizers.`

type tools struct {
	db  Reader
	sm  StateManager
	log *slog.Logger
}

func (t *tools) server(access Access, version string) *mcp.Server {
	instr := instructions
	if access == AccessWrite {
		instr += writeInstructions
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "insight", Title: "reconcile-kit insight", Version: version},
		&mcp.ServerOptions{Instructions: instr})

	ro := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: ptr(false)}
	addTool(t, s, access, &mcp.Tool{Name: "search_resources", Title: "Search resources", Annotations: ro,
		Description: "Find resources with filters, a label selector and filters on JSON fields; " +
			"returns one page of metadata (no spec/status) and the total count. " +
			"Same request as POST /insight/v1/resources/search."}, t.search)
	addTool(t, s, access, &mcp.Tool{Name: "list_values", Title: "List distinct values", Annotations: ro,
		Description: "Distinct values of resource_group, kind, namespace or shard_id among resources " +
			"matching the other filters of search. Kind values come with their resource_group. " +
			"Same request as POST /insight/v1/values."}, t.values)
	addTool(t, s, access, &mcp.Tool{Name: "get_resource", Title: "Get resource", Annotations: ro,
		Description: "Full resource (spec, status, labels, annotations, finalizers, versions) from state-manager."}, t.get)

	if access != AccessWrite {
		return s
	}
	addTool(t, s, access, &mcp.Tool{Name: "create_resource", Title: "Create resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)},
		Description: "Create a resource through state-manager. Fails if it already exists."}, t.create)
	addTool(t, s, access, &mcp.Tool{Name: "update_resource", Title: "Update resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)},
		Description: "Replace spec, labels, annotations and finalizers of a resource through state-manager. " +
			"Omitted labels/annotations/finalizers are cleared, so send the full set (get_resource first). " +
			"Pass version to fail with 409 on concurrent changes."}, t.update)
	addTool(t, s, access, &mcp.Tool{Name: "delete_resource", Title: "Delete resource",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)},
		Description: "Mark a resource for deletion through state-manager; it is removed once its finalizers are cleared."}, t.delete)
	return s
}

// addTool registers h and logs every call; calls on one resource are logged with it.
func addTool[In any](t *tools, s *mcp.Server, access Access, tool *mcp.Tool, h func(context.Context, In) (any, error)) {
	mcp.AddTool(s, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := h(ctx, in)
		attrs := []any{"tool", tool.Name, "access", access}
		if r, ok := any(in).(interface{ id() statemanager.ID }); ok {
			attrs = append(attrs, "target", r.id().String())
		}
		if err != nil {
			t.log.Warn("mcp tool failed", append(attrs, "err", err)...)
			return nil, nil, err
		}
		t.log.Info("mcp tool", attrs...)
		if raw, ok := out.(json.RawMessage); ok && len(raw) == 0 {
			out = map[string]any{"ok": true}
		}
		return nil, out, nil
	})
}

func ptr[T any](v T) *T { return &v }

// ---- read tools: the request shapes of /insight/v1 (query.Search), with schema docs ----

type FiltersIn struct {
	Name          string `json:"name,omitempty" jsonschema:"case-insensitive substring of the resource name"`
	ResourceGroup string `json:"resource_group,omitempty" jsonschema:"exact resource group"`
	Kind          string `json:"kind,omitempty" jsonschema:"exact kind"`
	Namespace     string `json:"namespace,omitempty" jsonschema:"exact namespace"`
	ShardID       string `json:"shard_id,omitempty" jsonschema:"exact shard id"`
	Pending       bool   `json:"pending,omitempty" jsonschema:"only resources whose spec is not reconciled yet (version > current_version)"`
	Deleting      bool   `json:"deleting,omitempty" jsonschema:"only resources marked for deletion"`
}

type JSONFilterIn struct {
	Root  string `json:"root" jsonschema:"JSON column: spec, status, annotations or finalizers"`
	Path  string `json:"path,omitempty" jsonschema:"path inside root, e.g. exit_code, resources.cpu, items[0].name, [\"app.kubernetes.io/name\"]; empty = the whole value"`
	Op    string `json:"op" jsonschema:"eq, ne, gt, gte, lt, lte, contains, not_contains, matches (case-insensitive regex), in (comma separated value), exists, not_exists"`
	Value string `json:"value,omitempty" jsonschema:"value to compare with; numbers compare numerically, JSON literals (true, null, \"0\") are accepted. Not used by exists/not_exists"`
}

type SortIn struct {
	Field string `json:"field,omitempty" jsonschema:"name, kind, resource_group, namespace, shard_id, created_at, updated_at or version; default newest first"`
	Desc  bool   `json:"desc,omitempty" jsonschema:"descending"`
}

type SearchIn struct {
	Filters   FiltersIn      `json:"filters,omitempty"`
	Labels    string         `json:"labels,omitempty" jsonschema:"label selector, comma separated terms: app=nginx, tier!=db, env in (a,b), env notin (c), key, !key, app~ngi (substring)"`
	JSON      []JSONFilterIn `json:"json,omitempty" jsonschema:"filters on JSON fields of spec/status/annotations/finalizers"`
	JSONMatch string         `json:"json_match,omitempty" jsonschema:"all (default): every JSON filter must match; any: at least one of them"`
	Sort      SortIn         `json:"sort,omitempty"`
	Limit     int            `json:"limit,omitempty" jsonschema:"page size, default 100, max 1000"`
	Offset    int            `json:"offset,omitempty" jsonschema:"number of matches to skip"`
}

func (in SearchIn) query() (query.Search, error) {
	f := in.Filters
	s := query.Search{
		Filters: query.Filters{
			Name: f.Name, ResourceGroup: f.ResourceGroup, Kind: f.Kind, Namespace: f.Namespace,
			ShardID: f.ShardID, Pending: f.Pending, Deleting: f.Deleting,
		},
		Labels:    in.Labels,
		JSONMatch: in.JSONMatch,
		Sort:      query.Sort{Field: in.Sort.Field, Desc: in.Sort.Desc},
		Limit:     in.Limit,
		Offset:    in.Offset,
	}
	for i, c := range in.JSON {
		// The UI skips half-filled rows; an agent must not silently get a broader result.
		if c.Op != query.OpExists && c.Op != query.OpNotExists && strings.TrimSpace(c.Value) == "" {
			return s, fmt.Errorf("json filter #%d: value is required for op %q", i+1, c.Op)
		}
		s.JSON = append(s.JSON, query.JSONFilter{Root: c.Root, Path: c.Path, Op: c.Op, Value: c.Value})
	}
	return s, nil
}

func (t *tools) search(ctx context.Context, in SearchIn) (any, error) {
	s, err := in.query()
	if err != nil {
		return nil, err
	}
	return t.db.List(ctx, s)
}

type ValuesIn struct {
	Field  string   `json:"field" jsonschema:"resource_group, kind, namespace or shard_id"`
	Q      string   `json:"q,omitempty" jsonschema:"case-insensitive substring of the value"`
	Search SearchIn `json:"search,omitempty" jsonschema:"only resources matching this search (its filter on field itself is ignored; sort and paging are ignored)"`
	Limit  int      `json:"limit,omitempty" jsonschema:"max values, default 200, max 500"`
}

func (t *tools) values(ctx context.Context, in ValuesIn) (any, error) {
	s, err := in.Search.query()
	if err != nil {
		return nil, err
	}
	vals, err := t.db.Values(ctx, in.Field, in.Q, s, query.ValuesLimit(in.Limit))
	if err != nil {
		return nil, err
	}
	return map[string]any{"values": vals}, nil
}

// ---- state-manager tools: path parameters plus the state-manager request body ----

type IDIn struct {
	ResourceGroup string `json:"resource_group"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
}

func (in IDIn) id() statemanager.ID {
	return statemanager.ID{ResourceGroup: in.ResourceGroup, Namespace: in.Namespace, Kind: in.Kind, Name: in.Name}
}

func (t *tools) get(ctx context.Context, in IDIn) (any, error) {
	return t.sm.Get(ctx, in.id())
}

type WriteIn struct {
	IDIn
	ShardID     string            `json:"shard_id" jsonschema:"shard (controller instance) that owns the resource"`
	Spec        map[string]any    `json:"spec,omitempty" jsonschema:"desired state (required by update_resource: the full spec)"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Finalizers  []string          `json:"finalizers,omitempty"`
}

func (t *tools) create(ctx context.Context, in WriteIn) (any, error) {
	return t.sm.Create(ctx, in.id(), map[string]any{
		"name": in.Name, "shard_id": in.ShardID, "spec": orEmpty(in.Spec),
		"labels": in.Labels, "annotations": in.Annotations, "finalizers": in.Finalizers,
	})
}

type UpdateIn struct {
	WriteIn
	Version *int `json:"version,omitempty" jsonschema:"version read by get_resource; the update fails with 409 if the resource changed since"`
}

func (t *tools) update(ctx context.Context, in UpdateIn) (any, error) {
	// An omitted spec would be sent as {} and wipe the desired state.
	if in.Spec == nil {
		return nil, fmt.Errorf("spec is required: send the full spec (get_resource first)")
	}
	body := map[string]any{
		"shard_id": in.ShardID, "spec": in.Spec,
		"labels": in.Labels, "annotations": in.Annotations, "finalizers": in.Finalizers,
	}
	if in.Version != nil {
		body["version"] = *in.Version
	}
	return t.sm.Update(ctx, in.id(), body)
}

func (t *tools) delete(ctx context.Context, in IDIn) (any, error) {
	if err := t.sm.Delete(ctx, in.id()); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "note": "marked for deletion; removed once finalizers are cleared"}, nil
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
