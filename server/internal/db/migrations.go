package db

import (
	"context"
	"fmt"

	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

const initialMigration = "0001_init.sql"

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migrations transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('beneco_tracker_migrations'))`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	var applied bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`,
		initialMigration,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check migration: %w", err)
	}
	if !applied {
		sql, err := migrations.Files.ReadFile(initialMigration)
		if err != nil {
			return fmt.Errorf("read migration: %w", err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, initialMigration,
		); err != nil {
			return fmt.Errorf("record migration: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
