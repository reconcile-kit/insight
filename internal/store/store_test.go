package store

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/reconcile-kit/insight/internal/query"
)

// testSchema isolates the fixture from the real tables of the stand's database.
const testSchema = "insight_test"

// Schema copied from state-manager migrations.
const schema = `
DROP SCHEMA IF EXISTS insight_test CASCADE;
CREATE SCHEMA insight_test;
SET search_path = insight_test;
CREATE TABLE resources (
	id SERIAL PRIMARY KEY,
	shard_id VARCHAR(255) NOT NULL DEFAULT 'default',
	resource_group VARCHAR(255) NOT NULL,
	kind VARCHAR(255) NOT NULL,
	namespace VARCHAR(255) NOT NULL,
	name VARCHAR(255) NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
	deletion_timestamp TIMESTAMP WITH TIME ZONE DEFAULT NULL,
	finalizers JSONB, annotations JSONB, spec JSONB, status JSONB,
	version BIGINT NOT NULL DEFAULT 1,
	current_version BIGINT NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ON resources (resource_group, kind, namespace, name);
CREATE TABLE labels (
	id SERIAL PRIMARY KEY,
	resource_id INTEGER NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
	name VARCHAR(255) NOT NULL,
	value VARCHAR(255) NOT NULL
);
INSERT INTO resources (shard_id, resource_group, kind, namespace, name, spec, status, annotations, finalizers, version, current_version, deletion_timestamp) VALUES
 ('s1','jobs.io','Job','ns1','job-ok',     '{"image":"nginx:1.27","replicas":3}', '{"exit_code":0,"phase":"Done"}',   '{"app.kubernetes.io/name":"web"}', '["cleanup"]', 2, 2, NULL),
 ('s1','jobs.io','Job','ns1','job-failed', '{"image":"busybox","replicas":1}',   '{"exit_code":1,"phase":"Failed"}', '{}', NULL, 3, 2, NULL),
 ('s2','jobs.io','Job','ns2','job-str',    '{"image":"alpine","replicas":"10"}', '{"exit_code":"137"}',              NULL, NULL, 1, 1, now()),
 ('s2','vm.io',  'VM', 'ns1','vm-a',       '{"cpu":4,"disks":[{"size":10}]}',   NULL,                                NULL, NULL, 1, 0, NULL);
INSERT INTO labels (resource_id, name, value) VALUES
 (1,'app','web'),(1,'env','prod'),(2,'app','batch'),(2,'env','dev'),(3,'env','prod'),(4,'team','core');
`

func setup(t *testing.T) *Store {
	t.Helper()
	// e.g. the e2e-tests stand: postgres://e2e:e2e@localhost:55432/e2e?sslmode=disable
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), dsn)
		if err == nil {
			_, _ = c.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE")
			_ = c.Close(context.Background())
		}
	})
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	s, err := New(ctx, Config{DSN: dsn + sep + "search_path=" + testSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func names(t *testing.T, s *Store, req query.Search) []string {
	t.Helper()
	res, err := s.List(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range res.Items {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	if res.Total == nil || int(*res.Total) != len(out) {
		t.Fatalf("total %v != %d (%s)", res.Total, len(out), res.CountError)
	}
	return out
}

func TestSearch(t *testing.T) {
	s := setup(t)
	j := func(root, path, op, val string) []query.JSONFilter {
		return []query.JSONFilter{{Root: root, Path: path, Op: op, Value: val}}
	}
	cases := []struct {
		name string
		req  query.Search
		want string
	}{
		{"all", query.Search{}, "job-failed,job-ok,job-str,vm-a"},
		{"name contains", query.Search{Filters: query.Filters{Name: "JOB-"}}, "job-failed,job-ok,job-str"},
		{"kind+ns", query.Search{Filters: query.Filters{Kind: "Job", Namespace: "ns1"}}, "job-failed,job-ok"},
		{"pending", query.Search{Filters: query.Filters{Pending: true}}, "job-failed,vm-a"},
		{"deleting", query.Search{Filters: query.Filters{Deleting: true}}, "job-str"},
		{"exit_code != 0", query.Search{JSON: j("status", "exit_code", "ne", "0")}, "job-failed,job-str"},
		{"exit_code = 0", query.Search{JSON: j("status", "exit_code", "eq", "0")}, "job-ok"},
		{"exit_code = string 137", query.Search{JSON: j("status", "exit_code", "eq", "137")}, "job-str"},
		{"phase = Failed unquoted", query.Search{JSON: j("status", "phase", "eq", "Failed")}, "job-failed"},
		{"replicas > 2 numeric incl. string", query.Search{JSON: j("spec", "replicas", "gt", "2")}, "job-ok,job-str"},
		{"image contains", query.Search{JSON: j("spec", "image", "contains", "NGINX")}, "job-ok"},
		{"image not contains", query.Search{JSON: j("spec", "image", "not_contains", "nginx")}, "job-failed,job-str,vm-a"},
		{"image matches", query.Search{JSON: j("spec", "image", "matches", "^(busy|alp)")}, "job-failed,job-str"},
		{"in", query.Search{JSON: j("status", "phase", "in", "Done, Failed")}, "job-failed,job-ok"},
		{"exists", query.Search{JSON: j("spec", "cpu", "exists", "")}, "vm-a"},
		{"not exists", query.Search{JSON: j("status", "", "not_exists", "")}, "vm-a"},
		{"array index", query.Search{JSON: j("spec", "disks[0].size", "gte", "10")}, "vm-a"},
		{"dotted key", query.Search{JSON: j("annotations", `["app.kubernetes.io/name"]`, "eq", "web")}, "job-ok"},
		{"finalizers contains", query.Search{JSON: j("finalizers", "", "contains", "cleanup")}, "job-ok"},
		{"incomplete row ignored", query.Search{JSON: j("spec", "image", "contains", "")}, "job-failed,job-ok,job-str,vm-a"},
		{"json any", query.Search{JSONMatch: "any", JSON: []query.JSONFilter{
			{Root: "spec", Path: "cpu", Op: "exists"}, {Root: "status", Path: "phase", Op: "eq", Value: "Done"}}}, "job-ok,vm-a"},
		{"label eq", query.Search{Labels: "env=prod"}, "job-ok,job-str"},
		{"label ne", query.Search{Labels: "env!=prod"}, "job-failed,vm-a"},
		{"label in + exists", query.Search{Labels: "env in (dev,prod), app"}, "job-failed,job-ok"},
		{"label notin", query.Search{Labels: "env notin (prod)"}, "job-failed,vm-a"},
		{"label missing", query.Search{Labels: "!env"}, "vm-a"},
		{"label substring", query.Search{Labels: "app~BAT"}, "job-failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(names(t, s, c.req), ","); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}

func TestPageSortLabels(t *testing.T) {
	s := setup(t)
	res, err := s.List(context.Background(), query.Search{Sort: query.Sort{Field: "name", Desc: true}, Limit: 2, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || res.Items[0].Name != "job-str" || res.Items[1].Name != "job-ok" || *res.Total != 4 {
		t.Fatalf("unexpected page %+v total %v", res.Items, *res.Total)
	}
	if res.Items[1].Labels["env"] != "prod" || res.Items[1].Labels["app"] != "web" {
		t.Fatalf("labels %+v", res.Items[1].Labels)
	}
}

func TestValuesAndPaths(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	v, err := s.Values(ctx, "kind", "", query.Search{Filters: query.Filters{Kind: "VM", Namespace: "ns2"}}, 50)
	if err != nil {
		t.Fatal(err)
	}
	// kind itself is ignored, namespace=ns2 applies.
	if strings.Join(v, ",") != "Job" {
		t.Fatalf("values %v", v)
	}
	v, _ = s.Values(ctx, "resource_group", "VM", query.Search{}, 50)
	if strings.Join(v, ",") != "vm.io" {
		t.Fatalf("values q %v", v)
	}
	paths, err := s.Paths(ctx, "spec", query.Search{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range paths {
		got = append(got, p.Path+":"+p.Type)
	}
	if strings.Join(got, " ") != "cpu:number disks:array image:string replicas:number" {
		t.Fatalf("paths %v", got)
	}
	ap, _ := s.Paths(ctx, "annotations", query.Search{}, 100)
	if len(ap) != 1 || ap[0].Path != `["app.kubernetes.io/name"]` {
		t.Fatalf("annotation paths %+v", ap)
	}
}

func TestUserErrors(t *testing.T) {
	s := setup(t)
	_, err := s.List(context.Background(), query.Search{JSON: []query.JSONFilter{{Root: "spec", Path: "image", Op: "matches", Value: "(("}}})
	var ue *UserError
	if err == nil || !asUser(err, &ue) {
		t.Fatalf("expected user error, got %v", err)
	}
}

func asUser(err error, target **UserError) bool {
	u, ok := err.(*UserError)
	*target = u
	return ok
}

func TestWritesAreRejected(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `DELETE FROM resources`); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
	err := s.readOnly(ctx, func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, `WITH d AS (DELETE FROM resources RETURNING 1) SELECT count(*) FROM d`).Scan(&n)
	})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
}
