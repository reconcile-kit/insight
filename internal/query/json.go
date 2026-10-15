package query

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// jsonRoots whitelists the jsonb columns that can be filtered.
var jsonRoots = map[string]struct{}{
	"spec":        {},
	"status":      {},
	"annotations": {},
	"finalizers":  {},
}

// JSONFilter is one row of the filter constructor: <root>.<path> <op> <value>.
type JSONFilter struct {
	Root  string `json:"root"`
	Path  string `json:"path"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

const (
	OpEq          = "eq"
	OpNe          = "ne"
	OpGt          = "gt"
	OpGte         = "gte"
	OpLt          = "lt"
	OpLte         = "lte"
	OpContains    = "contains"
	OpNotContains = "not_contains"
	OpMatches     = "matches"
	OpIn          = "in"
	OpExists      = "exists"
	OpNotExists   = "not_exists"
)

// valueOps maps each operator to whether an empty value makes the filter incomplete
// (and thus ignored). To compare with an empty string use the value `""`.
var valueOps = map[string]bool{
	OpExists: false, OpNotExists: false,
	OpEq: true, OpNe: true, OpGt: true, OpGte: true, OpLt: true, OpLte: true,
	OpContains: true, OpNotContains: true, OpMatches: true, OpIn: true,
}

var cmpOps = map[string]string{OpGt: ">", OpGte: ">=", OpLt: "<", OpLte: "<="}

var numericText = `'^\s*-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?\s*$'`

// ParsePath splits a path like `a.b[0].c` or `labels["app.kubernetes.io/name"]`
// into its segments. An empty path addresses the whole document.
func ParsePath(p string) ([]string, error) {
	p = strings.TrimSpace(p)
	segs := []string{}
	i := 0
	expectSep := false
	for i < len(p) {
		switch c := p[i]; {
		case c == '.':
			if !expectSep {
				return nil, fmt.Errorf("unexpected '.' at %d", i)
			}
			expectSep = false
			i++
			if i == len(p) {
				return nil, fmt.Errorf("path ends with '.'")
			}
		case c == '[':
			end := strings.IndexByte(p[i:], ']')
			inner := ""
			if i+1 < len(p) && p[i+1] == '"' {
				// Quoted key: find the closing quote honoring escapes.
				j := i + 2
				for j < len(p) && p[j] != '"' {
					if p[j] == '\\' {
						j++
					}
					j++
				}
				if j+1 >= len(p) || p[j+1] != ']' {
					return nil, fmt.Errorf("unterminated [\"...\"] at %d", i)
				}
				if err := json.Unmarshal([]byte(p[i+1:j+1]), &inner); err != nil {
					return nil, fmt.Errorf("bad quoted key at %d: %w", i, err)
				}
				end = j + 1 - i
			} else {
				if end < 0 {
					return nil, fmt.Errorf("unterminated '[' at %d", i)
				}
				inner = strings.TrimSpace(p[i+1 : i+end])
				if _, err := strconv.Atoi(inner); err != nil {
					return nil, fmt.Errorf("array index must be a number or a quoted key at %d", i)
				}
			}
			segs = append(segs, inner)
			i += end + 1
			expectSep = true
		default:
			if expectSep {
				return nil, fmt.Errorf("expected '.' or '[' at %d", i)
			}
			j := i
			for j < len(p) && p[j] != '.' && p[j] != '[' {
				j++
			}
			segs = append(segs, strings.TrimSpace(p[i:j]))
			i = j
			expectSep = true
		}
	}
	return segs, nil
}

var plainKey = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

// FormatPath is the inverse of ParsePath.
func FormatPath(segs []string) string {
	var sb strings.Builder
	for i, s := range segs {
		if plainKey.MatchString(s) {
			if i > 0 {
				sb.WriteByte('.')
			}
			sb.WriteString(s)
			continue
		}
		q, _ := json.Marshal(s)
		sb.WriteByte('[')
		sb.Write(q)
		sb.WriteByte(']')
	}
	return sb.String()
}

// literal returns the value both as a jsonb literal and as the text that
// #>> would yield for it, so `0`, `"0"`, `Running` and `"Running"` all work.
func literal(v string) (jsonLit, text string) {
	v = strings.TrimSpace(v)
	if json.Valid([]byte(v)) {
		var s string
		if json.Unmarshal([]byte(v), &s) == nil {
			return v, s
		}
		return v, v
	}
	q, _ := json.Marshal(v)
	return string(q), v
}

// jsonCond returns the SQL condition for f, or "" when f is incomplete.
func (b *builder) jsonCond(f *JSONFilter) (string, error) {
	if _, ok := jsonRoots[f.Root]; !ok {
		return "", fmt.Errorf("unknown root %q", f.Root)
	}
	segs, err := ParsePath(f.Path)
	if err != nil {
		return "", err
	}
	if _, ok := valueOps[f.Op]; !ok {
		return "", fmt.Errorf("unknown operator %q", f.Op)
	}
	if valueOps[f.Op] && strings.TrimSpace(f.Value) == "" {
		return "", nil
	}
	p := b.arg(segs) + "::text[]"
	x := "(r." + f.Root + " #> " + p + ")"
	t := "(r." + f.Root + " #>> " + p + ")"

	switch f.Op {
	case OpExists:
		return x + " IS NOT NULL", nil
	case OpNotExists:
		return x + " IS NULL", nil
	}

	eq := func(v string) string {
		j, s := literal(v)
		return "(" + x + " = " + b.arg(j) + "::jsonb OR " + t + " = " + b.arg(s) + ")"
	}

	switch f.Op {
	case OpEq:
		return eq(f.Value), nil
	case OpNe:
		return "(" + x + " IS NOT NULL AND NOT " + eq(f.Value) + ")", nil
	case OpIn:
		var parts []string
		for _, v := range strings.Split(f.Value, ",") {
			if strings.TrimSpace(v) != "" {
				parts = append(parts, eq(v))
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return "(" + strings.Join(parts, " OR ") + ")", nil
	case OpGt, OpGte, OpLt, OpLte:
		op := cmpOps[f.Op]
		_, s := literal(f.Value)
		if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			n := b.arg(strings.TrimSpace(s)) + "::numeric"
			return "(CASE WHEN jsonb_typeof(" + x + ") = 'number' THEN " + x + "::numeric " + op + " " + n +
				" WHEN jsonb_typeof(" + x + ") = 'string' AND " + t + " ~ " + numericText + " THEN " + t + "::numeric " + op + " " + n +
				" END)", nil
		}
		return "(" + t + " " + op + " " + b.arg(s) + ")", nil
	case OpContains:
		_, s := literal(f.Value)
		return "(" + t + " ILIKE " + b.arg("%"+escapeLike(s)+"%") + ")", nil
	case OpNotContains:
		_, s := literal(f.Value)
		return "(" + t + " IS NULL OR " + t + " NOT ILIKE " + b.arg("%"+escapeLike(s)+"%") + ")", nil
	case OpMatches:
		return "(" + t + " ~* " + b.arg(strings.TrimSpace(f.Value)) + ")", nil
	default:
		return "", fmt.Errorf("unknown operator %q", f.Op)
	}
}
