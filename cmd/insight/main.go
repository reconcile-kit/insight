package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reconcile-kit/insight/internal/config"
	"github.com/reconcile-kit/insight/internal/mcpserver"
	"github.com/reconcile-kit/insight/internal/server"
	"github.com/reconcile-kit/insight/internal/statemanager"
	"github.com/reconcile-kit/insight/internal/store"
	"github.com/reconcile-kit/insight/web"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("insight stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, store.Config{
		DSN:              cfg.DatabaseDSN,
		MaxConns:         cfg.DBMaxConns,
		StatementTimeout: cfg.StatementTimeout,
	})
	if err != nil {
		return err
	}
	defer db.Close()

	var mcpHandler http.Handler
	if mc := (mcpserver.Config{
		ReadTokens:  cfg.MCPReadTokens,
		WriteTokens: cfg.MCPWriteTokens,
		Version:     version,
		Log:         log,
	}); mc.Enabled() {
		mcpHandler = mcpserver.Handler(mc, db, statemanager.New(cfg.StateManagerURL, cfg.StateManagerToken))
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: server.New(server.Config{
			StateManagerURL: cfg.StateManagerURL,
			Static:          web.FS(),
			Log:             log,
			MCP:             mcpHandler,
		}, db),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr, "state_manager", cfg.StateManagerURL.String(),
			"mcp_read_tokens", len(cfg.MCPReadTokens), "mcp_write_tokens", len(cfg.MCPWriteTokens))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
