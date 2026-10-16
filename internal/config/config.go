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
	return c, nil
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
