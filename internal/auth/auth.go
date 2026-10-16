// Package auth implements browser login for insight through any OpenID Connect
// provider (Keycloak, Dex, Okta, Entra ID, Google, ...).
//
// Flow: authorization code with PKCE, state and nonce kept in a short-lived
// signed cookie. After the callback insight issues its own HMAC-signed session
// cookie with the user's name, email and access level; the provider's tokens
// are not kept (except the ID token, in a separate cookie scoped to /auth, as
// id_token_hint for logout).
//
// Access levels come from roles in the token, see Access and Config.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Access is what a user may do. Levels are ordered: each includes the previous.
type Access int

const (
	AccessNone  Access = iota // logged in, but no role grants access
	AccessRead                // list, search, open resources
	AccessWrite               // create and update
	AccessAdmin               // delete
)

func (a Access) String() string {
	switch a {
	case AccessRead:
		return "read"
	case AccessWrite:
		return "write"
	case AccessAdmin:
		return "admin"
	}
	return "none"
}

func (a Access) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }

func (a *Access) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "read":
		*a = AccessRead
	case "write":
		*a = AccessWrite
	case "admin":
		*a = AccessAdmin
	default:
		*a = AccessNone
	}
	return nil
}

type Config struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// PublicURL is where users open insight; the redirect URI is PublicURL + /auth/callback.
	PublicURL *url.URL
	// SessionSecret signs the session and state cookies (at least 32 bytes).
	SessionSecret []byte
	SessionTTL    time.Duration
	CookieSecure  bool

	// RolesClaim is a claim (dotted path, e.g. "groups" or "realm_access.roles")
	// with the user's roles. Keycloak roles are collected as well: realm roles
	// as "<role>", client roles as "<client>:<role>".
	RolesClaim string
	// ReadRoles, WriteRoles and AdminRoles grant the levels; any of the listed
	// roles is enough. An empty list does not restrict that level: empty
	// ReadRoles lets every logged-in user read, empty WriteRoles lets everyone
	// who may read also write, empty AdminRoles lets writers delete.
	ReadRoles  []string
	WriteRoles []string
	AdminRoles []string

	Log *slog.Logger
}

// User is the identity kept in the session.
type User struct {
	Subject string `json:"sub"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name,omitempty"`
	Access  Access `json:"access"`
	Expires int64  `json:"exp"`
}

const (
	SessionCookie = "insight_session"
	stateCookie   = "insight_oidc_state"
	idTokenCookie = "insight_id_token"
	// maxCookie keeps cookies under the 4 KB browser limit.
	maxCookie = 3800
)

type Auth struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	provider *providerState
}

type providerState struct {
	oauth      oauth2.Config
	verifier   *oidc.IDTokenVerifier
	endSession string
}

func New(cfg Config) (*Auth, error) {
	if len(cfg.SessionSecret) < 32 {
		return nil, errors.New("session secret must be at least 32 bytes")
	}
	if cfg.PublicURL == nil {
		return nil, errors.New("public URL is required")
	}
	if cfg.IssuerURL == "" || cfg.ClientID == "" {
		return nil, errors.New("issuer URL and client id are required")
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "groups"
	}
	return &Auth{cfg: cfg, log: cfg.Log}, nil
}

// discover fetches the provider metadata once; a failure is retried on the
// next request, so insight starts even while the provider is unavailable.
func (a *Auth) discover(ctx context.Context) (*providerState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.provider != nil {
		return a.provider, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(ctx, a.cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	var meta struct {
		EndSession string `json:"end_session_endpoint"`
	}
	_ = p.Claims(&meta)
	a.provider = &providerState{
		oauth: oauth2.Config{
			ClientID:     a.cfg.ClientID,
			ClientSecret: a.cfg.ClientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  a.cfg.PublicURL.JoinPath("auth/callback").String(),
			Scopes:       a.cfg.Scopes,
		},
		verifier:   p.Verifier(&oidc.Config{ClientID: a.cfg.ClientID}),
		endSession: meta.EndSession,
	}
	return a.provider, nil
}

// ---- signed cookie values: base64url(json) "." base64url(hmac-sha256) ----

func (a *Auth) sign(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	return payload + "." + base64.RawURLEncoding.EncodeToString(a.mac(payload)), nil
}

func (a *Auth) verify(s string, v any) bool {
	payload, sig, ok := strings.Cut(s, ".")
	if !ok {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, a.mac(payload)) {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	return err == nil && json.Unmarshal(b, v) == nil
}

func (a *Auth) mac(payload string) []byte {
	m := hmac.New(sha256.New, a.cfg.SessionSecret)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

func randToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Auth) setCookie(w http.ResponseWriter, name, value, path string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: path, MaxAge: maxAge,
		Secure: a.cfg.CookieSecure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// SessionValue returns the signed session cookie value for u.
func (a *Auth) SessionValue(u User) (string, error) { return a.sign(u) }

// UserFrom returns the user of a valid, unexpired session cookie.
func (a *Auth) UserFrom(r *http.Request) (*User, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil, false
	}
	var u User
	if !a.verify(c.Value, &u) || time.Now().Unix() >= u.Expires {
		return nil, false
	}
	return &u, true
}

// ---- handlers ----

type loginState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
	Expires  int64  `json:"e"`
}

// SafeReturnTo keeps only local paths, so a login link cannot redirect elsewhere.
func SafeReturnTo(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") ||
		strings.HasPrefix(p, "/login") || strings.HasPrefix(p, "/auth/") {
		return "/"
	}
	return p
}

// Login starts the authorization code flow: GET /auth/login?return_to=/path.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	p, err := a.discover(r.Context())
	if err != nil {
		a.log.Error("auth: provider unavailable", "err", err)
		a.fail(w, r, "provider_unavailable")
		return
	}
	st := loginState{
		State:    randToken(),
		Nonce:    randToken(),
		Verifier: oauth2.GenerateVerifier(),
		ReturnTo: SafeReturnTo(r.URL.Query().Get("return_to")),
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	}
	v, err := a.sign(st)
	if err != nil {
		a.fail(w, r, "internal")
		return
	}
	a.setCookie(w, stateCookie, v, "/auth", 600)
	http.Redirect(w, r, p.oauth.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)), http.StatusFound)
}

// Callback finishes the flow and sets the session: GET /auth/callback.
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := r.Cookie(stateCookie)
	a.setCookie(w, stateCookie, "", "/auth", -1) // one-shot
	var st loginState
	if err != nil || !a.verify(c.Value, &st) || time.Now().Unix() >= st.Expires {
		a.fail(w, r, "state_expired")
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		a.fail(w, r, "state_mismatch")
		return
	}
	if e := q.Get("error"); e != "" {
		a.log.Warn("auth: provider returned an error", "error", e, "description", q.Get("error_description"))
		a.fail(w, r, "provider_error")
		return
	}
	p, err := a.discover(r.Context())
	if err != nil {
		a.log.Error("auth: provider unavailable", "err", err)
		a.fail(w, r, "provider_unavailable")
		return
	}
	tok, err := p.oauth.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		a.log.Error("auth: code exchange failed", "err", err)
		a.fail(w, r, "exchange_failed")
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := p.verifier.Verify(r.Context(), rawID)
	if err != nil {
		a.log.Error("auth: invalid id token", "err", err)
		a.fail(w, r, "invalid_token")
		return
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(st.Nonce)) != 1 {
		a.fail(w, r, "invalid_token")
		return
	}

	claims := map[string]any{}
	if err := idt.Claims(&claims); err != nil {
		a.fail(w, r, "invalid_token")
		return
	}
	// Keycloak puts roles into the access token only (by default). It came
	// straight from the token endpoint over TLS, so its claims are trusted.
	if at, ok := jwtClaims(tok.AccessToken); ok {
		for k, v := range at {
			if _, exists := claims[k]; !exists {
				claims[k] = v
			}
		}
	}

	roles := Roles(claims, a.cfg.RolesClaim)
	u := User{
		Subject: idt.Subject,
		Email:   str(claims["email"]),
		Name:    firstNonEmpty(str(claims["name"]), str(claims["preferred_username"]), str(claims["email"]), idt.Subject),
		Access:  a.access(roles),
		Expires: time.Now().Add(a.cfg.SessionTTL).Unix(),
	}
	v, err := a.SessionValue(u)
	if err != nil {
		a.fail(w, r, "internal")
		return
	}
	ttl := int(a.cfg.SessionTTL.Seconds())
	a.setCookie(w, SessionCookie, v, "/", ttl)
	if len(rawID) <= maxCookie {
		a.setCookie(w, idTokenCookie, rawID, "/auth", ttl)
	}
	a.log.Info("auth: login", "user", u.Email, "name", u.Name, "access", u.Access.String(), "roles", roles)
	http.Redirect(w, r, st.ReturnTo, http.StatusFound)
}

// Logout clears the session and ends the provider session (RP-initiated
// logout) when the provider supports it: GET /auth/logout.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	a.setCookie(w, SessionCookie, "", "/", -1)
	a.setCookie(w, idTokenCookie, "", "/auth", -1)
	after := a.cfg.PublicURL.JoinPath("login").String()
	if p, err := a.discover(r.Context()); err == nil && p.endSession != "" {
		u, err := url.Parse(p.endSession)
		if err == nil {
			v := u.Query()
			v.Set("client_id", a.cfg.ClientID)
			v.Set("post_logout_redirect_uri", after)
			if c, err := r.Cookie(idTokenCookie); err == nil && c.Value != "" {
				v.Set("id_token_hint", c.Value)
			}
			u.RawQuery = v.Encode()
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

// fail sends the browser back to the login page with an error code it can show.
func (a *Auth) fail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+url.QueryEscape(code), http.StatusFound)
}

func (a *Auth) access(roles []string) Access {
	has := func(want []string) bool {
		for _, w := range want {
			for _, r := range roles {
				if strings.EqualFold(w, r) {
					return true
				}
			}
		}
		return false
	}
	c := a.cfg
	read := len(c.ReadRoles) == 0 || has(c.ReadRoles) || has(c.WriteRoles) || has(c.AdminRoles)
	write := has(c.WriteRoles) || has(c.AdminRoles) || (len(c.WriteRoles) == 0 && read)
	admin := has(c.AdminRoles) || (len(c.AdminRoles) == 0 && write)
	switch {
	case admin:
		return AccessAdmin
	case write:
		return AccessWrite
	case read:
		return AccessRead
	}
	return AccessNone
}

// Roles collects role names from claim (a dotted path to a list or a comma
// separated string) and from Keycloak's realm_access / resource_access.
// Leading "/" of Keycloak group paths is dropped.
func Roles(claims map[string]any, claim string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimPrefix(strings.TrimSpace(s), "/")
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range strList(lookup(claims, claim)) {
		add(s)
	}
	for _, s := range strList(lookup(claims, "realm_access.roles")) {
		add(s)
	}
	if ra, ok := claims["resource_access"].(map[string]any); ok {
		for client, v := range ra {
			if m, ok := v.(map[string]any); ok {
				for _, s := range strList(m["roles"]) {
					add(client + ":" + s)
				}
			}
		}
	}
	return out
}

func lookup(m map[string]any, path string) any {
	var cur any = m
	for _, p := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func strList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.Split(t, ",")
	}
	return nil
}

// jwtClaims decodes the payload of a JWT without verifying it.
func jwtClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	m := map[string]any{}
	return m, json.Unmarshal(b, &m) == nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
