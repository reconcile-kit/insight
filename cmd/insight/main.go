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
	"github.com/reconcile-kit/insight/internal/server"
	"github.com/reconcile-kit/insight/internal/store"
	"github.com/reconcile-kit/insight/web"
)

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

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: server.New(server.Config{
			StateManagerURL: cfg.StateManagerURL,
			Static:          web.FS(),
			Log:             log,
		}, db),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr, "state_manager", cfg.StateManagerURL.String())
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
