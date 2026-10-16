package statemanager

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClient(t *testing.T) {
	type seen struct{ method, path, auth, body string }
	var got []seen
	sm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, seen{r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), string(b)})
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":"Version conflict: resource version not match"}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(w, `{"name":"x"}`)
		}
	}))
	defer sm.Close()
	u, _ := url.Parse(sm.URL + "/prefix")
	c := New(u, "sm-token")
	ctx := context.Background()
	id := ID{ResourceGroup: "a.example.io", Namespace: "ns", Kind: "Job", Name: "x y"}

	if b, err := c.Get(ctx, id); err != nil || string(b) != `{"name":"x"}` {
		t.Fatalf("get: %s %v", b, err)
	}
	if _, err := c.Create(ctx, ID{ResourceGroup: "g", Namespace: "ns", Kind: "Job"}, map[string]any{"name": "n"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	var se *Error
	if _, err := c.Update(ctx, id, map[string]any{}); !errors.As(err, &se) || se.Status != 409 || se.Message != "Version conflict: resource version not match" {
		t.Fatalf("update error = %v", err)
	}
	if err := c.Delete(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	want := []seen{
		{"GET", "/prefix/api/v1/groups/a.example.io/namespaces/ns/kinds/Job/resources/x%20y", "Bearer sm-token", ""},
		{"POST", "/prefix/api/v1/groups/g/namespaces/ns/kinds/Job/resources/", "Bearer sm-token", `{"name":"n"}`},
		{"PUT", "/prefix/api/v1/groups/a.example.io/namespaces/ns/kinds/Job/resources/x%20y", "Bearer sm-token", `{}`},
		{"DELETE", "/prefix/api/v1/groups/a.example.io/namespaces/ns/kinds/Job/resources/x%20y", "Bearer sm-token", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("requests = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Ids that would escape their path segment are rejected before any request.
	for _, bad := range []ID{
		{ResourceGroup: "g", Namespace: "ns", Kind: "Job", Name: "../../resources"},
		{ResourceGroup: "g", Namespace: "..", Kind: "Job", Name: "x"},
		{ResourceGroup: "g", Namespace: "ns", Kind: "", Name: "x"},
	} {
		if _, err := c.Get(ctx, bad); err == nil {
			t.Errorf("get %+v: no error", bad)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("bad ids reached state-manager")
	}
}
