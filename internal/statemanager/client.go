// Package statemanager is a minimal client for the single-resource API of
// state-manager. Insight uses it where it acts on its own behalf (the MCP
// endpoint); the UI talks to state-manager through the transparent proxy.
package statemanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

// New returns a client for base. A non-empty token is sent as a bearer token
// (needed when state-manager runs with AUTH_ENABLED).
func New(base *url.URL, token string) *Client {
	return &Client{base: base, token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

// ID addresses one resource.
type ID struct {
	ResourceGroup string `json:"resource_group"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
}

func (id ID) String() string {
	return id.ResourceGroup + "/" + id.Namespace + "/" + id.Kind + "/" + id.Name
}

func (id ID) validate(withName bool) error {
	var missing []string
	for _, f := range []struct{ k, v string }{
		{"resource_group", id.ResourceGroup}, {"namespace", id.Namespace}, {"kind", id.Kind},
	} {
		if strings.TrimSpace(f.v) == "" {
			missing = append(missing, f.k)
		}
	}
	if withName && strings.TrimSpace(id.Name) == "" {
		missing = append(missing, "name")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required: %s", strings.Join(missing, ", "))
	}
	// Each part becomes one path segment of the state-manager URL.
	for _, v := range []string{id.ResourceGroup, id.Namespace, id.Kind, id.Name} {
		if strings.Contains(v, "/") || v == "." || v == ".." {
			return fmt.Errorf("invalid resource id part %q", v)
		}
	}
	return nil
}

// Error is a non-2xx answer of state-manager.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("state-manager %d: %s", e.Status, e.Message) }

func (c *Client) path(id ID, withName bool) string {
	p := c.base.JoinPath("api/v1/groups", id.ResourceGroup, "namespaces", id.Namespace, "kinds", id.Kind, "resources")
	if withName {
		p = p.JoinPath(id.Name)
	} else {
		p.Path += "/"
		p.RawPath = ""
	}
	return p.String()
}

// Get returns the full resource.
func (c *Client) Get(ctx context.Context, id ID) (json.RawMessage, error) {
	if err := id.validate(true); err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodGet, c.path(id, true), nil)
}

// Create posts body (shard_id, name, spec, labels, ...) into the collection of id.
func (c *Client) Create(ctx context.Context, id ID, body any) (json.RawMessage, error) {
	if err := id.validate(false); err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, c.path(id, false), body)
}

// Update replaces the spec, labels, annotations and finalizers of id.
func (c *Client) Update(ctx context.Context, id ID, body any) (json.RawMessage, error) {
	if err := id.validate(true); err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPut, c.path(id, true), body)
}

// Delete marks id for deletion (removed once its finalizers are gone).
func (c *Client) Delete(ctx context.Context, id ID) error {
	if err := id.validate(true); err != nil {
		return err
	}
	_, err := c.do(ctx, http.MethodDelete, c.path(id, true), nil)
	return err
}

func (c *Client) do(ctx context.Context, method, u string, body any) (json.RawMessage, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("state-manager unavailable: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return nil, &Error{Status: res.StatusCode, Message: msg}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	return data, nil
}
