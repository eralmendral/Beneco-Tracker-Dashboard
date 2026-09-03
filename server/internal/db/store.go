package db

import (
	"context"
	"fmt"

	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Health(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) IngestBarangayFeeders(ctx context.Context, records []model.BarangayFeeder) (model.ScrapeRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.ScrapeRun{}, fmt.Errorf("begin barangay ingest: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	run, err := createRun(ctx, tx, model.SourceBarangayFeeders)
	if err != nil {
		return model.ScrapeRun{}, err
	}
	rows := make([][]any, len(records))
	for i, record := range records {
		rows[i] = []any{run.ID, record.BarangayID, record.Barangay, record.Municipality, record.Feeder}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"barangay_feeders"},
		[]string{"scrape_run_id", "barangayid", "barangay", "municipality", "feeder"},
		pgx.CopyFromRows(rows),
	); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("insert barangay records: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("commit barangay ingest: %w", err)
	}
	return run, nil
}

func (s *Store) IngestContractors(ctx context.Context, records []model.Contractor) (model.ScrapeRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.ScrapeRun{}, fmt.Errorf("begin contractor ingest: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	run, err := createRun(ctx, tx, model.SourceContractors)
	if err != nil {
		return model.ScrapeRun{}, err
	}
	rows := make([][]any, len(records))
	for i, record := range records {
		rows[i] = []any{run.ID, record.SourceNo, record.Company, nullable(record.Address), nullable(record.Business), nullable(record.Contact), nullable(record.PRC)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"contractors"},
		[]string{"scrape_run_id", "source_no", "company", "address", "business", "contact", "prc"},
		pgx.CopyFromRows(rows),
	); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("insert contractor records: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("commit contractor ingest: %w", err)
	}
	return run, nil
}

func createRun(ctx context.Context, tx pgx.Tx, source string) (model.ScrapeRun, error) {
	var run model.ScrapeRun
	err := tx.QueryRow(ctx,
		`INSERT INTO scrape_runs (source) VALUES ($1) RETURNING id, source, scraped_at`, source,
	).Scan(&run.ID, &run.Source, &run.ScrapedAt)
	if err != nil {
		return model.ScrapeRun{}, fmt.Errorf("create scrape run: %w", err)
	}
	return run, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) ListScrapeRuns(ctx context.Context, source string) ([]model.ScrapeRun, error) {
	query := `SELECT id, source, scraped_at FROM scrape_runs`
	args := []any{}
	if source != "" {
		query += ` WHERE source = $1`
		args = append(args, source)
	}
	query += ` ORDER BY scraped_at DESC, id DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list scrape runs: %w", err)
	}
	defer rows.Close()

	runs := make([]model.ScrapeRun, 0)
	for rows.Next() {
		var run model.ScrapeRun
		if err := rows.Scan(&run.ID, &run.Source, &run.ScrapedAt); err != nil {
			return nil, fmt.Errorf("scan scrape run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scrape runs: %w", err)
	}
	return runs, nil
}
