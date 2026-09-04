package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/model"
	"github.com/jackc/pgx/v5"
)

var ErrSnapshotNotFound = errors.New("snapshot not found")

func (s *Store) LatestBarangayFeeders(ctx context.Context) ([]model.BarangayFeeder, error) {
	runID, err := s.latestRunID(ctx, model.SourceBarangayFeeders)
	if err != nil {
		return nil, err
	}
	return s.BarangayFeedersByRun(ctx, runID)
}

func (s *Store) OutagesSince(ctx context.Context, since time.Time) ([]model.OutageEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source_id, feeder, area, COALESCE(cause, ''), started_at,
		       restored_at, duration_minutes, status, source_url
		FROM outage_events
		WHERE started_at >= $1
		ORDER BY started_at DESC, source_id, feeder`, since)
	if err != nil {
		return nil, fmt.Errorf("query outages: %w", err)
	}
	defer rows.Close()

	records := make([]model.OutageEvent, 0)
	for rows.Next() {
		var record model.OutageEvent
		if err := rows.Scan(&record.SourceID, &record.Feeder, &record.Area, &record.Cause,
			&record.StartedAt, &record.RestoredAt, &record.DurationMinutes, &record.Status, &record.SourceURL); err != nil {
			return nil, fmt.Errorf("scan outage: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outages: %w", err)
	}
	return records, nil
}

func (s *Store) FacebookReportsSince(ctx context.Context, since time.Time) ([]model.FacebookReport, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source_id, post_url, reported_at, COALESCE(location, ''), feeder, comment_excerpt, category
		FROM facebook_reports
		WHERE reported_at >= $1
		ORDER BY reported_at DESC, source_id, feeder`, since)
	if err != nil {
		return nil, fmt.Errorf("query Facebook reports: %w", err)
	}
	defer rows.Close()

	records := make([]model.FacebookReport, 0)
	for rows.Next() {
		var record model.FacebookReport
		if err := rows.Scan(&record.SourceID, &record.PostURL, &record.ReportedAt, &record.Location, &record.Feeder, &record.CommentExcerpt, &record.Category); err != nil {
			return nil, fmt.Errorf("scan Facebook report: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Facebook reports: %w", err)
	}
	return records, nil
}

func (s *Store) BarangayFeedersByRun(ctx context.Context, runID int64) ([]model.BarangayFeeder, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT bf.barangayid, bf.barangay, bf.municipality, bf.feeder
		FROM barangay_feeders bf
		JOIN scrape_runs sr ON sr.id = bf.scrape_run_id
		WHERE bf.scrape_run_id = $1 AND sr.source = $2
		ORDER BY bf.id`, runID, model.SourceBarangayFeeders)
	if err != nil {
		return nil, fmt.Errorf("query barangay snapshot: %w", err)
	}
	defer rows.Close()

	records := make([]model.BarangayFeeder, 0)
	for rows.Next() {
		var record model.BarangayFeeder
		if err := rows.Scan(&record.BarangayID, &record.Barangay, &record.Municipality, &record.Feeder); err != nil {
			return nil, fmt.Errorf("scan barangay snapshot: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate barangay snapshot: %w", err)
	}
	if len(records) == 0 {
		return nil, ErrSnapshotNotFound
	}
	return records, nil
}

func (s *Store) LatestContractors(ctx context.Context) ([]model.Contractor, error) {
	runID, err := s.latestRunID(ctx, model.SourceContractors)
	if err != nil {
		return nil, err
	}
	return s.ContractorsByRun(ctx, runID)
}

func (s *Store) ContractorsByRun(ctx context.Context, runID int64) ([]model.Contractor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.source_no, c.company, COALESCE(c.address, ''),
		       COALESCE(c.business, ''), COALESCE(c.contact, ''), COALESCE(c.prc, '')
		FROM contractors c
		JOIN scrape_runs sr ON sr.id = c.scrape_run_id
		WHERE c.scrape_run_id = $1 AND sr.source = $2
		ORDER BY c.id`, runID, model.SourceContractors)
	if err != nil {
		return nil, fmt.Errorf("query contractor snapshot: %w", err)
	}
	defer rows.Close()

	records := make([]model.Contractor, 0)
	for rows.Next() {
		var record model.Contractor
		if err := rows.Scan(&record.SourceNo, &record.Company, &record.Address, &record.Business, &record.Contact, &record.PRC); err != nil {
			return nil, fmt.Errorf("scan contractor snapshot: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contractor snapshot: %w", err)
	}
	if len(records) == 0 {
		return nil, ErrSnapshotNotFound
	}
	return records, nil
}

func (s *Store) latestRunID(ctx context.Context, source string) (int64, error) {
	var runID int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM scrape_runs
		WHERE source = $1
		ORDER BY scraped_at DESC, id DESC
		LIMIT 1`, source).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrSnapshotNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find latest %s run: %w", source, err)
	}
	return runID, nil
}
