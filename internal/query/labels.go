package query

import (
	"fmt"
	"regexp"
	"strings"
)

// LabelTerm is one parsed term of a label selector.
type LabelTerm struct {
	Key    string
	Op     string // "=", "!=", "~", "in", "notin", "exists", "!exists"
	Values []string
}

var (
	setTerm = regexp.MustCompile(`^(.+?)\s+(in|notin)\s*\((.*)\)$`)
	cmpTerm = regexp.MustCompile(`^(.+?)\s*(==|!=|=|~)\s*(.*)$`)
	// labelKey matches k8s-style keys: [prefix/]name.
	labelKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-/]*$`)
)

// splitTerms splits on commas that are not inside parentheses.
func splitTerms(s string) ([]string, error) {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced ')'")
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced '('")
	}
	return append(out, s[start:]), nil
}

// ParseLabels parses a selector such as
// `app=nginx, tier!=db, env in (a,b), env notin (c), team, !legacy, app~ngi`.
func ParseLabels(sel string) ([]LabelTerm, error) {
	parts, err := splitTerms(sel)
	if err != nil {
		return nil, err
	}
	var terms []LabelTerm
	for _, raw := range parts {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if m := setTerm.FindStringSubmatch(t); m != nil {
			var vals []string
			for _, v := range strings.Split(m[3], ",") {
				if v = strings.TrimSpace(v); v != "" {
					vals = append(vals, v)
				}
			}
			if len(vals) == 0 {
				return nil, fmt.Errorf("%q: empty value list", t)
			}
			terms = append(terms, LabelTerm{Key: strings.TrimSpace(m[1]), Op: m[2], Values: vals})
			continue
		}
		if m := cmpTerm.FindStringSubmatch(t); m != nil {
			op := m[2]
			if op == "==" {
				op = "="
			}
			key := strings.TrimSpace(m[1])
			if key == "" {
				return nil, fmt.Errorf("%q: missing key", t)
			}
			terms = append(terms, LabelTerm{Key: key, Op: op, Values: []string{strings.TrimSpace(m[3])}})
			continue
		}
		if strings.HasPrefix(t, "!") {
			key := strings.TrimSpace(t[1:])
			if key == "" {
				return nil, fmt.Errorf("%q: missing key", t)
			}
			terms = append(terms, LabelTerm{Key: key, Op: "!exists"})
			continue
		}
		if strings.ContainsAny(t, " ()") {
			return nil, fmt.Errorf("cannot parse %q", t)
		}
		terms = append(terms, LabelTerm{Key: t, Op: "exists"})
	}
	for _, t := range terms {
		if !labelKey.MatchString(t.Key) {
			return nil, fmt.Errorf("invalid label key %q", t.Key)
		}
	}
	return terms, nil
}

func (b *builder) applyLabels(sel string) error {
	terms, err := ParseLabels(sel)
	if err != nil {
		return fmt.Errorf("labels: %w", err)
	}
	for _, t := range terms {
		sub := "SELECT 1 FROM labels l WHERE l.resource_id = r.id AND l.name = " + b.arg(t.Key)
		switch t.Op {
		case "exists":
			b.conds = append(b.conds, "EXISTS ("+sub+")")
		case "!exists":
			b.conds = append(b.conds, "NOT EXISTS ("+sub+")")
		case "=":
			b.conds = append(b.conds, "EXISTS ("+sub+" AND l.value = "+b.arg(t.Values[0])+")")
		case "!=":
			// k8s semantics: resources without the key match too.
			b.conds = append(b.conds, "NOT EXISTS ("+sub+" AND l.value = "+b.arg(t.Values[0])+")")
		case "~":
			b.conds = append(b.conds, "EXISTS ("+sub+" AND l.value ILIKE "+b.arg("%"+escapeLike(t.Values[0])+"%")+")")
		case "in":
			b.conds = append(b.conds, "EXISTS ("+sub+" AND l.value = ANY("+b.arg(t.Values)+"::text[]))")
		case "notin":
			b.conds = append(b.conds, "NOT EXISTS ("+sub+" AND l.value = ANY("+b.arg(t.Values)+"::text[]))")
		}
	}
	return nil
}
