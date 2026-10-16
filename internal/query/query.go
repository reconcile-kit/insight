// Package query translates insight search requests into parameterized SQL
// against the state-manager schema (tables resources and labels).
//
// Every user supplied value is passed as a bind parameter; only whitelisted
// column names are ever interpolated into the SQL text.
package query

import (
	"fmt"
	"strings"
)

const (
	DefaultLimit = 100
	MaxLimit     = 1000

	DefaultValuesLimit = 200
	MaxValuesLimit     = 500
)

// ValuesLimit applies the default and the cap of the values method.
func ValuesLimit(n int) int {
	if n <= 0 || n > MaxValuesLimit {
		return DefaultValuesLimit
	}
	return n
}

// Filters are the top-level column filters.
type Filters struct {
	Name          string `json:"name"`
	ResourceGroup string `json:"resource_group"`
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace"`
	ShardID       string `json:"shard_id"`
	// Pending selects resources whose spec was not yet reconciled (version > current_version).
	Pending bool `json:"pending"`
	// Deleting selects resources marked for deletion (deletion_timestamp is set).
	Deleting bool `json:"deleting"`
}

// Search is the full request for the resource list.
type Search struct {
	Filters Filters `json:"filters"`
	// Labels is a k8s-like selector: "app=nginx, tier!=db, env in (a,b), key, !key, app~ngi".
	Labels string       `json:"labels"`
	JSON   []JSONFilter `json:"json"`
	// JSONMatch is "all" (default) or "any".
	JSONMatch string `json:"json_match"`
	Sort      Sort   `json:"sort"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type Sort struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc"`
}

// sortColumns whitelists sortable fields.
var sortColumns = map[string]string{
	"name":           "r.name",
	"kind":           "r.kind",
	"resource_group": "r.resource_group",
	"namespace":      "r.namespace",
	"shard_id":       "r.shard_id",
	"created_at":     "r.created_at",
	"updated_at":     "r.updated_at",
	"version":        "r.version",
}

// ValueFields whitelists the fields that can be listed with DISTINCT.
var ValueFields = map[string]string{
	"resource_group": "r.resource_group",
	"kind":           "r.kind",
	"namespace":      "r.namespace",
	"shard_id":       "r.shard_id",
}

// builder accumulates WHERE conditions and their bind arguments.
type builder struct {
	conds []string
	args  []any
}

func (b *builder) arg(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

func (b *builder) where() string {
	if len(b.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(b.conds, " AND ")
}

// escapeLike escapes LIKE wildcards so the value is matched literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// applyFilters adds the conditions for s. The field named skip is ignored,
// which lets a dropdown list values of a field filtered by everything else.
func (b *builder) applyFilters(s *Search, skip string) error {
	f := s.Filters
	if v := strings.TrimSpace(f.Name); v != "" && skip != "name" {
		b.conds = append(b.conds, "r.name ILIKE "+b.arg("%"+escapeLike(v)+"%"))
	}
	exact := []struct{ field, col, val string }{
		{"resource_group", "r.resource_group", f.ResourceGroup},
		{"kind", "r.kind", f.Kind},
		{"namespace", "r.namespace", f.Namespace},
		{"shard_id", "r.shard_id", f.ShardID},
	}
	for _, e := range exact {
		if v := strings.TrimSpace(e.val); v != "" && e.field != skip {
			b.conds = append(b.conds, e.col+" = "+b.arg(v))
		}
	}
	if f.Pending {
		b.conds = append(b.conds, "r.version > r.current_version")
	}
	if f.Deleting {
		b.conds = append(b.conds, "r.deletion_timestamp IS NOT NULL")
	}

	if err := b.applyLabels(s.Labels); err != nil {
		return err
	}

	var jsonConds []string
	for i := range s.JSON {
		c, err := b.jsonCond(&s.JSON[i])
		if err != nil {
			return fmt.Errorf("json filter #%d: %w", i+1, err)
		}
		if c != "" {
			jsonConds = append(jsonConds, c)
		}
	}
	if len(jsonConds) > 0 {
		switch s.JSONMatch {
		case "", "all":
			b.conds = append(b.conds, jsonConds...)
		case "any":
			b.conds = append(b.conds, "("+strings.Join(jsonConds, " OR ")+")")
		default:
			return fmt.Errorf("json_match must be all or any")
		}
	}
	return nil
}

func normalizePage(s *Search) {
	if s.Limit <= 0 {
		s.Limit = DefaultLimit
	}
	if s.Limit > MaxLimit {
		s.Limit = MaxLimit
	}
	if s.Offset < 0 {
		s.Offset = 0
	}
}

// ListColumns is the projection used by List, in scan order.
const listColumns = `r.shard_id, r.resource_group, r.kind, r.namespace, r.name,
	r.created_at, r.updated_at, r.deletion_timestamp, r.version, r.current_version,
	COALESCE((SELECT jsonb_object_agg(l.name, l.value) FROM labels l WHERE l.resource_id = r.id), '{}'::jsonb)`

// List builds the page query.
func List(s Search) (string, []any, error) {
	normalizePage(&s)
	b := &builder{}
	if err := b.applyFilters(&s, ""); err != nil {
		return "", nil, err
	}
	order := "r.id DESC"
	if s.Sort.Field != "" {
		col, ok := sortColumns[s.Sort.Field]
		if !ok {
			return "", nil, fmt.Errorf("unknown sort field %q", s.Sort.Field)
		}
		dir := "ASC"
		if s.Sort.Desc {
			dir = "DESC"
		}
		order = col + " " + dir + ", r.id " + dir
	}
	q := "SELECT " + listColumns + " FROM resources r" + b.where() +
		" ORDER BY " + order +
		" LIMIT " + b.arg(s.Limit) + " OFFSET " + b.arg(s.Offset)
	return q, b.args, nil
}

// Count builds the total count query for the same filters.
func Count(s Search) (string, []any, error) {
	b := &builder{}
	if err := b.applyFilters(&s, ""); err != nil {
		return "", nil, err
	}
	return "SELECT count(*) FROM resources r" + b.where(), b.args, nil
}

// Values builds a DISTINCT query of field, filtered by every other filter of s
// and by a case-insensitive substring q. It selects two columns: the value and,
// for kind, its resource_group (kinds are colored per group in the UI), else ”.
func Values(field, q string, s Search, limit int) (string, []any, error) {
	col, ok := ValueFields[field]
	if !ok {
		return "", nil, fmt.Errorf("unknown field %q", field)
	}
	b := &builder{}
	if err := b.applyFilters(&s, field); err != nil {
		return "", nil, err
	}
	if q = strings.TrimSpace(q); q != "" {
		b.conds = append(b.conds, col+" ILIKE "+b.arg("%"+escapeLike(q)+"%"))
	}
	group := "''::text"
	if field == "kind" {
		group = "r.resource_group"
	}
	return "SELECT DISTINCT " + col + ", " + group + " FROM resources r" + b.where() +
		" ORDER BY 1, 2 LIMIT " + b.arg(limit), b.args, nil
}
