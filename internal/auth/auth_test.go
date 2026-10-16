package auth

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func newAuth(t *testing.T, read, write, admin []string) *Auth {
	t.Helper()
	u, _ := url.Parse("https://insight.example.com")
	a, err := New(Config{
		IssuerURL: "https://idp.example.com", ClientID: "insight", PublicURL: u,
		SessionSecret: []byte(strings.Repeat("s", 32)),
		ReadRoles:     read, WriteRoles: write, AdminRoles: admin,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRolesKeycloak(t *testing.T) {
	claims := map[string]any{
		"groups":          []any{"/insight-readers", "ops"},
		"realm_access":    map[string]any{"roles": []any{"offline_access"}},
		"resource_access": map[string]any{"gatekeeper": map[string]any{"roles": []any{"insight-get", "insight-edit"}}},
	}
	got := Roles(claims, "groups")
	slices.Sort(got)
	want := []string{"gatekeeper:insight-edit", "gatekeeper:insight-get", "insight-readers", "offline_access", "ops"}
	if !slices.Equal(got, want) {
		t.Fatalf("roles = %v, want %v", got, want)
	}
	if got := Roles(map[string]any{"a": map[string]any{"b": "x, y"}}, "a.b"); !slices.Equal(got, []string{"x", "y"}) {
		t.Fatalf("dotted/comma claim = %v", got)
	}
}

func TestAccess(t *testing.T) {
	gk := newAuth(t, []string{"gatekeeper:insight-get"}, []string{"gatekeeper:insight-edit"}, []string{"gatekeeper:insight-admin"})
	for _, tc := range []struct {
		roles []string
		want  Access
	}{
		{nil, AccessNone},
		{[]string{"other"}, AccessNone},
		{[]string{"gatekeeper:insight-get"}, AccessRead},
		{[]string{"GATEKEEPER:INSIGHT-GET"}, AccessRead},
		{[]string{"gatekeeper:insight-edit"}, AccessWrite}, // a writer can read without the read role
		{[]string{"gatekeeper:insight-get", "gatekeeper:insight-admin"}, AccessAdmin},
	} {
		if got := gk.access(tc.roles); got != tc.want {
			t.Errorf("roles %v: access %v, want %v", tc.roles, got, tc.want)
		}
	}

	// Empty lists do not restrict their level.
	if got := newAuth(t, nil, nil, nil).access(nil); got != AccessAdmin {
		t.Errorf("no role lists: %v, want admin", got)
	}
	readOnlyByDefault := newAuth(t, nil, []string{"editors"}, nil)
	if got := readOnlyByDefault.access(nil); got != AccessRead {
		t.Errorf("anyone with write roles set: %v, want read", got)
	}
	if got := readOnlyByDefault.access([]string{"editors"}); got != AccessAdmin {
		t.Errorf("editor without admin roles: %v, want admin", got)
	}
}

func TestSessionCookie(t *testing.T) {
	a := newAuth(t, nil, nil, nil)
	v, err := a.sign(User{Subject: "s", Email: "u@example.com", Access: AccessWrite, Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := func(val string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: SessionCookie, Value: val})
		return r
	}
	if u, ok := a.UserFrom(req(v)); !ok || u.Email != "u@example.com" || u.Access != AccessWrite {
		t.Fatalf("valid cookie: %+v %v", u, ok)
	}

	// Tampering with the payload (e.g. raising access) breaks the signature.
	payload, sig, _ := strings.Cut(v, ".")
	forged, _ := a.sign(User{Subject: "s", Email: "u@example.com", Access: AccessAdmin, Expires: time.Now().Add(time.Hour).Unix()})
	forgedPayload, _, _ := strings.Cut(forged, ".")
	for _, bad := range []string{forgedPayload + "." + sig, payload + ".AAAA", payload, ""} {
		if _, ok := a.UserFrom(req(bad)); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	// Another secret does not accept it either.
	other := newAuth(t, nil, nil, nil)
	other.cfg.SessionSecret = []byte(strings.Repeat("x", 32))
	if _, ok := other.UserFrom(req(v)); ok {
		t.Error("accepted with a different secret")
	}
	// Expired sessions are rejected.
	old, _ := a.sign(User{Subject: "s", Access: AccessAdmin, Expires: time.Now().Add(-time.Second).Unix()})
	if _, ok := a.UserFrom(req(old)); ok {
		t.Error("accepted an expired session")
	}
}

func TestSafeReturnTo(t *testing.T) {
	for in, want := range map[string]string{
		"/?kind=Job":               "/?kind=Job",
		"/resources":               "/resources",
		"https://evil.example.com": "/",
		"//evil.example.com":       "/",
		"/\\evil.example.com":      "/",
		"/login?x=1":               "/",
		"/auth/logout":             "/",
		"":                         "/",
	} {
		if got := SafeReturnTo(in); got != want {
			t.Errorf("SafeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	a := newAuth(t, nil, nil, nil)
	// No state cookie at all.
	rec := httptest.NewRecorder()
	a.Callback(rec, httptest.NewRequest("GET", "/auth/callback?state=x&code=y", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?error=state_expired" {
		t.Fatalf("no cookie: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// Signed state that does not match the query.
	st, _ := a.sign(loginState{State: "right", Expires: time.Now().Add(time.Minute).Unix()})
	r := httptest.NewRequest("GET", "/auth/callback?state=wrong&code=y", nil)
	r.AddCookie(&http.Cookie{Name: stateCookie, Value: st})
	rec = httptest.NewRecorder()
	a.Callback(rec, r)
	if rec.Header().Get("Location") != "/login?error=state_mismatch" {
		t.Fatalf("state mismatch: %s", rec.Header().Get("Location"))
	}
}
