package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"dispatch/migrations"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrInvalidState = errors.New("invalid state transition")

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	config.MaxConns = 16
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (store *Store) Close()                          { store.pool.Close() }
func (store *Store) Ready(ctx context.Context) error { return store.pool.Ping(ctx) }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func requireChanged(command pgconnCommandTag) error {
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type pgconnCommandTag interface {
	RowsAffected() int64
}
