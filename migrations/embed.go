package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtext('dispatch_schema_migrations'))`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('dispatch_schema_migrations'))`)
	}()

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versionText := strings.SplitN(entry.Name(), "_", 2)[0]
		version, err := strconv.ParseInt(versionText, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid migration name %q", entry.Name())
		}
		content, err := files.ReadFile(entry.Name())
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, connection, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS schema_migrations (
					version BIGINT PRIMARY KEY,
					applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`); err != nil {
				return err
			}
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return nil
			}
			if _, err := tx.Exec(ctx, string(content)); err != nil {
				return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1) ON CONFLICT DO NOTHING`, version)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
