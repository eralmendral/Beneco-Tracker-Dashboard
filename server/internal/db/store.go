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

func (s *Store) IngestOutages(ctx context.Context, records []model.OutageEvent) (model.ScrapeRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.ScrapeRun{}, fmt.Errorf("begin outage ingest: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	run, err := createRun(ctx, tx, model.SourceOutages)
	if err != nil {
		return model.ScrapeRun{}, err
	}
	for _, record := range records {
		_, err = tx.Exec(ctx, `
			INSERT INTO outage_events (
				scrape_run_id, source_id, feeder, area, cause, started_at,
				restored_at, duration_minutes, status, source_url
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (source_id, feeder) DO UPDATE SET
				scrape_run_id = EXCLUDED.scrape_run_id,
				area = EXCLUDED.area,
				cause = EXCLUDED.cause,
				started_at = EXCLUDED.started_at,
				restored_at = EXCLUDED.restored_at,
				duration_minutes = EXCLUDED.duration_minutes,
				status = EXCLUDED.status,
				source_url = EXCLUDED.source_url,
				last_seen_at = now()`,
			run.ID, record.SourceID, record.Feeder, record.Area, nullable(record.Cause),
			record.StartedAt, record.RestoredAt, record.DurationMinutes, record.Status, record.SourceURL)
		if err != nil {
			return model.ScrapeRun{}, fmt.Errorf("upsert outage %s: %w", record.SourceID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("commit outage ingest: %w", err)
	}
	return run, nil
}

func (s *Store) IngestFacebookReports(ctx context.Context, records []model.FacebookReport) (model.ScrapeRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.ScrapeRun{}, fmt.Errorf("begin Facebook report ingest: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	run, err := createRun(ctx, tx, model.SourceFacebookReports)
	if err != nil {
		return model.ScrapeRun{}, err
	}
	for _, record := range records {
		_, err = tx.Exec(ctx, `
			INSERT INTO facebook_reports (
				scrape_run_id, source_id, post_url, reported_at, location, feeder, comment_excerpt, category
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (source_id, feeder) DO UPDATE SET
				scrape_run_id = EXCLUDED.scrape_run_id,
				post_url = EXCLUDED.post_url,
				reported_at = EXCLUDED.reported_at,
				location = EXCLUDED.location,
				comment_excerpt = EXCLUDED.comment_excerpt,
				category = EXCLUDED.category,
				last_seen_at = now()`,
			run.ID, record.SourceID, record.PostURL, record.ReportedAt,
			nullable(record.Location), record.Feeder, record.CommentExcerpt, record.Category)
		if err != nil {
			return model.ScrapeRun{}, fmt.Errorf("upsert Facebook report %s: %w", record.SourceID, err)
		}
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM facebook_reports AS unmapped
		WHERE unmapped.feeder = 'UNMAPPED'
		  AND EXISTS (
			SELECT 1 FROM facebook_reports AS mapped
			WHERE mapped.source_id = unmapped.source_id
			  AND mapped.feeder <> 'UNMAPPED'
		  )`); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("remove superseded unmapped Facebook reports: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ScrapeRun{}, fmt.Errorf("commit Facebook report ingest: %w", err)
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
