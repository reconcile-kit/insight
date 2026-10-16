// Package config reads insight settings from the environment.
//
// Database settings use the same variables as state-manager, so the same
// secret can be reused (ideally with a read-only role / replica host).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr       string
	StateManagerURL  *url.URL
	DatabaseDSN      string
	DBMaxConns       int32
	StatementTimeout time.Duration

	// MCPReadTokens and MCPWriteTokens enable /mcp; see internal/mcpserver.
	MCPReadTokens  []string
	MCPWriteTokens []string
	// StateManagerToken is sent to state-manager by the MCP tools (the UI
	// proxy forwards the caller's own Authorization header instead).
	StateManagerToken string

	// Auth is nil when login is disabled (AUTH_ENABLED is not true).
	Auth *Auth
}

// Auth is the OIDC login configuration, see internal/auth.
type Auth struct {
	IssuerURL     string
	ClientID      string
	ClientSecret  string
	Scopes        []string
	PublicURL     *url.URL
	SessionSecret []byte
	SessionTTL    time.Duration
	CookieSecure  bool
	RolesClaim    string
	ReadRoles     []string
	WriteRoles    []string
	AdminRoles    []string
}

// minTokenLen keeps obviously guessable MCP tokens out.
const minTokenLen = 16

func Load() (*Config, error) {
	c := &Config{
		ListenAddr: env("LISTEN_ADDR", ":27182"),
	}

	if raw := os.Getenv("STATE_MANAGER_URL"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("STATE_MANAGER_URL must be an absolute URL, got %q", raw)
		}
		c.StateManagerURL = u
	} else {
		return nil, errors.New("STATE_MANAGER_URL is required")
	}

	dsn, err := databaseDSN()
	if err != nil {
		return nil, err
	}
	c.DatabaseDSN = dsn

	maxConns, err := strconv.Atoi(env("DATABASE_MAX_CONNS", "10"))
	if err != nil {
		return nil, fmt.Errorf("DATABASE_MAX_CONNS: %w", err)
	}
	c.DBMaxConns = int32(maxConns)

	c.StatementTimeout, err = time.ParseDuration(env("DATABASE_STATEMENT_TIMEOUT", "15s"))
	if err != nil {
		return nil, fmt.Errorf("DATABASE_STATEMENT_TIMEOUT: %w", err)
	}

	if c.MCPReadTokens, err = tokens("MCP_READ_TOKEN"); err != nil {
		return nil, err
	}
	if c.MCPWriteTokens, err = tokens("MCP_WRITE_TOKEN"); err != nil {
		return nil, err
	}
	for _, r := range c.MCPReadTokens {
		for _, w := range c.MCPWriteTokens {
			if r == w {
				return nil, errors.New("MCP_READ_TOKEN and MCP_WRITE_TOKEN must not share a token")
			}
		}
	}
	c.StateManagerToken = os.Getenv("STATE_MANAGER_TOKEN")

	if c.Auth, err = loadAuth(); err != nil {
		return nil, err
	}
	return c, nil
}

func loadAuth() (*Auth, error) {
	enabled, err := strconv.ParseBool(env("AUTH_ENABLED", "false"))
	if err != nil {
		return nil, fmt.Errorf("AUTH_ENABLED: %w", err)
	}
	if !enabled {
		return nil, nil
	}
	a := &Auth{
		IssuerURL:     os.Getenv("AUTH_OIDC_ISSUER_URL"),
		ClientID:      os.Getenv("AUTH_OIDC_CLIENT_ID"),
		ClientSecret:  os.Getenv("AUTH_OIDC_CLIENT_SECRET"),
		Scopes:        list(env("AUTH_OIDC_SCOPES", "openid,email,profile")),
		SessionSecret: []byte(os.Getenv("AUTH_SESSION_SECRET")),
		RolesClaim:    env("AUTH_ROLES_CLAIM", "groups"),
		ReadRoles:     list(os.Getenv("AUTH_READ_ROLES")),
		WriteRoles:    list(os.Getenv("AUTH_WRITE_ROLES")),
		AdminRoles:    list(os.Getenv("AUTH_ADMIN_ROLES")),
	}
	var missing []string
	for _, v := range []struct{ k, v string }{
		{"AUTH_OIDC_ISSUER_URL", a.IssuerURL}, {"AUTH_OIDC_CLIENT_ID", a.ClientID},
		{"AUTH_SESSION_SECRET", string(a.SessionSecret)}, {"PUBLIC_URL", os.Getenv("PUBLIC_URL")},
	} {
		if v.v == "" {
			missing = append(missing, v.k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("AUTH_ENABLED=true requires %s", strings.Join(missing, ", "))
	}
	if len(a.SessionSecret) < 32 {
		return nil, errors.New("AUTH_SESSION_SECRET must be at least 32 characters (e.g. openssl rand -hex 32)")
	}
	u, err := url.Parse(os.Getenv("PUBLIC_URL"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("PUBLIC_URL must be an absolute URL, got %q", os.Getenv("PUBLIC_URL"))
	}
	a.PublicURL = u
	if a.SessionTTL, err = time.ParseDuration(env("AUTH_SESSION_TTL", "12h")); err != nil {
		return nil, fmt.Errorf("AUTH_SESSION_TTL: %w", err)
	}
	// Secure cookies by default whenever insight is served over https.
	if a.CookieSecure, err = strconv.ParseBool(env("AUTH_COOKIE_SECURE", strconv.FormatBool(u.Scheme == "https"))); err != nil {
		return nil, fmt.Errorf("AUTH_COOKIE_SECURE: %w", err)
	}
	return a, nil
}

func list(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// tokens reads a comma separated list of tokens (several allow rotation).
func tokens(key string) ([]string, error) {
	var out []string
	for _, t := range strings.Split(os.Getenv(key), ",") {
		if t = strings.TrimSpace(t); t == "" {
			continue
		}
		if len(t) < minTokenLen {
			return nil, fmt.Errorf("%s: tokens must be at least %d characters", key, minTokenLen)
		}
		out = append(out, t)
	}
	return out, nil
}

// databaseDSN prefers DATABASE_URL and otherwise builds the DSN from
// DATABASE_HOST/PORT/USERNAME/PASSWORD/NAME/SSLMODE like state-manager.
func databaseDSN() (string, error) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn, nil
	}
	user, pass, name := os.Getenv("DATABASE_USERNAME"), os.Getenv("DATABASE_PASSWORD"), os.Getenv("DATABASE_NAME")
	if user == "" || name == "" {
		return "", errors.New("set DATABASE_URL or DATABASE_USERNAME, DATABASE_PASSWORD and DATABASE_NAME")
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     env("DATABASE_HOST", "localhost") + ":" + env("DATABASE_PORT", "5432"),
		Path:     "/" + name,
		RawQuery: url.Values{"sslmode": {env("DATABASE_SSLMODE", "disable")}}.Encode(),
	}
	return u.String(), nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
