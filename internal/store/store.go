// Package store runs read-only queries against the state-manager database.
//
// Writes are impossible by construction: every session is opened with
// default_transaction_read_only=on and every query runs inside an explicit
// READ ONLY transaction. Point it at a replica or a read-only role anyway.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/reconcile-kit/insight/internal/query"
)

type Config struct {
	DSN              string
	MaxConns         int32
	StatementTimeout time.Duration
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	pc, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if cfg.MaxConns > 0 {
		pc.MaxConns = cfg.MaxConns
	}
	rp := pc.ConnConfig.RuntimeParams
	rp["default_transaction_read_only"] = "on"
	rp["application_name"] = "insight"
	if cfg.StatementTimeout > 0 {
		rp["statement_timeout"] = strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// readOnly runs fn inside a READ ONLY transaction that is always rolled back.
func (s *Store) readOnly(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	return fn(tx)
}

// UserError marks errors caused by the request (bad filter, bad regex, ...).
type UserError struct{ Err error }

func (e *UserError) Error() string { return e.Err.Error() }
func (e *UserError) Unwrap() error { return e.Err }

// classify turns Postgres data exceptions (class 22, e.g. invalid regular
// expression or numeric cast) into user errors.
func classify(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && len(pe.Code) == 5 && pe.Code[:2] == "22" {
		return &UserError{Err: errors.New(pe.Message)}
	}
	return err
}

type Resource struct {
	ShardID           string            `json:"shard_id"`
	ResourceGroup     string            `json:"resource_group"`
	Kind              string            `json:"kind"`
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	CreatedAt         *time.Time        `json:"created_at"`
	UpdatedAt         *time.Time        `json:"updated_at"`
	DeletionTimestamp *time.Time        `json:"deletion_timestamp"`
	Version           int64             `json:"version"`
	CurrentVersion    int64             `json:"current_version"`
	Labels            map[string]string `json:"labels"`
}

type ListResult struct {
	Items []Resource `json:"items"`
	// Total is nil when counting failed (e.g. hit the statement timeout).
	Total      *int64 `json:"total"`
	CountError string `json:"count_error,omitempty"`
}

// List returns one page and, concurrently, the total number of matches.
func (s *Store) List(ctx context.Context, req query.Search) (*ListResult, error) {
	lq, largs, err := query.List(req)
	if err != nil {
		return nil, &UserError{Err: err}
	}
	cq, cargs, err := query.Count(req)
	if err != nil {
		return nil, &UserError{Err: err}
	}

	type countRes struct {
		n   int64
		err error
	}
	countCh := make(chan countRes, 1)
	go func() {
		var n int64
		err := s.readOnly(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, cq, cargs...).Scan(&n) })
		countCh <- countRes{n, err}
	}()

	res := &ListResult{Items: []Resource{}}
	err = s.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, lq, largs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Resource
			var labels []byte
			if err := rows.Scan(&r.ShardID, &r.ResourceGroup, &r.Kind, &r.Namespace, &r.Name,
				&r.CreatedAt, &r.UpdatedAt, &r.DeletionTimestamp, &r.Version, &r.CurrentVersion, &labels); err != nil {
				return err
			}
			if err := json.Unmarshal(labels, &r.Labels); err != nil {
				return err
			}
			res.Items = append(res.Items, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, classify(err)
	}

	c := <-countCh
	if c.err != nil {
		res.CountError = c.err.Error()
	} else {
		res.Total = &c.n
	}
	return res, nil
}

type Value struct {
	Value string `json:"value"`
	// Group is set for kind values: the same kind may exist in several groups.
	Group string `json:"group,omitempty"`
}

// Values lists distinct values of field matching the other filters.
func (s *Store) Values(ctx context.Context, field, q string, req query.Search, limit int) ([]Value, error) {
	sq, args, err := query.Values(field, q, req, limit)
	if err != nil {
		return nil, &UserError{Err: err}
	}
	out := []Value{}
	err = s.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sq, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Value, error) {
			var v Value
			err := row.Scan(&v.Value, &v.Group)
			return v, err
		})
		return err
	})
	return out, classify(err)
}

type Path struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// Paths lists JSON paths of root found in a sample of matching resources.
func (s *Store) Paths(ctx context.Context, root string, req query.Search, sample int) ([]Path, error) {
	sq, args, err := query.Paths(root, req, sample)
	if err != nil {
		return nil, &UserError{Err: err}
	}
	out := []Path{}
	err = s.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sq, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var segs []string
			var p Path
			if err := rows.Scan(&segs, &p.Type, &p.Count); err != nil {
				return err
			}
			p.Path = query.FormatPath(segs)
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, classify(err)
}
