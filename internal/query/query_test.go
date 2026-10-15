package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePath(t *testing.T) {
	cases := map[string][]string{
		"":                           {},
		"exit_code":                  {"exit_code"},
		"a.b.c":                      {"a", "b", "c"},
		"conditions[0].type":         {"conditions", "0", "type"},
		`["app.kubernetes.io/name"]`: {"app.kubernetes.io/name"},
		`meta["a.b"].c`:              {"meta", "a.b", "c"},
		`x["quo\"te"]`:               {"x", `quo"te`},
	}
	for in, want := range cases {
		got, err := ParsePath(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	for _, bad := range []string{".a", "a.", "a..b", "a[x]", `a["x`, "a[0"} {
		if _, err := ParsePath(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestParseLabels(t *testing.T) {
	got, err := ParseLabels(`app=nginx, tier != db, env in (a, b), env notin (c), team, !legacy, app~ngi, x==y`)
	if err != nil {
		t.Fatal(err)
	}
	want := []LabelTerm{
		{"app", "=", []string{"nginx"}},
		{"tier", "!=", []string{"db"}},
		{"env", "in", []string{"a", "b"}},
		{"env", "notin", []string{"c"}},
		{"team", "exists", nil},
		{"legacy", "!exists", nil},
		{"app", "~", []string{"ngi"}},
		{"x", "=", []string{"y"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	for _, bad := range []string{"env in (a", "a b", "env in ()", "=x"} {
		if _, err := ParseLabels(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestListIsParameterized(t *testing.T) {
	evil := `x'; DROP TABLE resources; --`
	q, args, err := List(Search{
		Filters: Filters{Name: evil, Kind: evil},
		Labels:  "app=" + strings.ReplaceAll(evil, ",", ""),
		JSON:    []JSONFilter{{Root: "status", Path: evil, Op: OpEq, Value: evil}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q, "DROP") {
		t.Fatalf("value leaked into SQL: %s", q)
	}
	if len(args) == 0 {
		t.Fatal("no args")
	}
	if _, _, err := List(Search{JSON: []JSONFilter{{Root: "spec; drop", Op: OpExists}}}); err == nil {
		t.Fatal("expected root whitelist error")
	}
	if _, _, err := List(Search{Sort: Sort{Field: "id; drop"}}); err == nil {
		t.Fatal("expected sort whitelist error")
	}
}
