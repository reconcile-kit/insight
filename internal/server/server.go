// Package server wires the insight HTTP API:
//
//   - /insight/v1/*         complex read queries, served directly from the DB (read-only)
//   - /api/v1/groups/*      single-resource GET and all writes, proxied to state-manager as is
//   - everything else       the embedded SPA
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/reconcile-kit/insight/internal/query"
	"github.com/reconcile-kit/insight/internal/store"
)

type Reader interface {
	List(ctx context.Context, req query.Search) (*store.ListResult, error)
	Values(ctx context.Context, field, q string, req query.Search, limit int) ([]store.Value, error)
	Ping(ctx context.Context) error
}

type Config struct {
	StateManagerURL *url.URL
	Static          fs.FS
	Log             *slog.Logger
}

const maxBody = 1 << 20

func New(cfg Config, db Reader) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, requestLogger(cfg.Log), middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody(err))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	h := &handlers{db: db, log: cfg.Log}
	r.Route("/insight/v1", func(r chi.Router) {
		r.Post("/resources/search", h.search)
		r.Post("/values", h.values)
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown insight method"})
		})
	})

	// Only the single-resource routes of state-manager are exposed; its list
	// endpoint is intentionally not reachable through insight.
	proxy := newProxy(cfg.StateManagerURL, cfg.Log)
	r.Handle("/api/v1/groups/*", proxy)
	r.Handle("/api/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not exposed by insight; use /insight/v1 for reads"})
	}))

	r.Handle("/*", spa(cfg.Static))
	return r
}

type handlers struct {
	db  Reader
	log *slog.Logger
}

func (h *handlers) search(w http.ResponseWriter, r *http.Request) {
	var req query.Search
	if !decode(w, r, &req) {
		return
	}
	res, err := h.db.List(r.Context(), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type valuesReq struct {
	Field  string       `json:"field"`
	Q      string       `json:"q"`
	Search query.Search `json:"search"`
	Limit  int          `json:"limit"`
}

func (h *handlers) values(w http.ResponseWriter, r *http.Request) {
	var req valuesReq
	if !decode(w, r, &req) {
		return
	}
	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 200
	}
	vals, err := h.db.Values(r.Context(), req.Field, req.Q, req.Search, req.Limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"values": vals})
}

func (h *handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ue *store.UserError
	if errors.As(err, &ue) {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	h.log.Error("query failed", "path", r.URL.Path, "err", err, "request_id", middleware.GetReqID(r.Context()))
	writeJSON(w, http.StatusInternalServerError, errBody(err))
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return false
	}
	return true
}

func errBody(err error) map[string]string { return map[string]string{"error": err.Error()} }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func newProxy(target *url.URL, log *slog.Logger) http.Handler {
	if target == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "STATE_MANAGER_URL is not configured"})
		})
	}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("state-manager proxy", "path", r.URL.Path, "err", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "state-manager unavailable: " + err.Error()})
		},
	}
}

// spa serves static files and falls back to index.html for client routes.
func spa(static fs.FS) http.Handler {
	files := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := static.Open(p); err == nil {
				_ = f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.Error(w, "frontend is not built", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"bytes", ww.BytesWritten(), "duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()))
		})
	}
}
