package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/db"
	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/model"
)

type fakeStore struct {
	healthErr           error
	contractorRun       model.ScrapeRun
	ingestedContractors []model.Contractor
	latestBarangays     []model.BarangayFeeder
	latestBarangayErr   error
}

func (f *fakeStore) Health(context.Context) error { return f.healthErr }

func (f *fakeStore) IngestBarangayFeeders(context.Context, []model.BarangayFeeder) (model.ScrapeRun, error) {
	return model.ScrapeRun{}, errors.New("unexpected call")
}

func (f *fakeStore) IngestContractors(_ context.Context, records []model.Contractor) (model.ScrapeRun, error) {
	f.ingestedContractors = records
	return f.contractorRun, nil
}

func (f *fakeStore) ListScrapeRuns(context.Context, string) ([]model.ScrapeRun, error) {
	return []model.ScrapeRun{}, nil
}

func (f *fakeStore) LatestBarangayFeeders(context.Context) ([]model.BarangayFeeder, error) {
	return f.latestBarangays, f.latestBarangayErr
}

func (f *fakeStore) BarangayFeedersByRun(context.Context, int64) ([]model.BarangayFeeder, error) {
	return []model.BarangayFeeder{}, nil
}

func (f *fakeStore) LatestContractors(context.Context) ([]model.Contractor, error) {
	return []model.Contractor{}, nil
}

func (f *fakeStore) ContractorsByRun(context.Context, int64) ([]model.Contractor, error) {
	return []model.Contractor{}, nil
}

func testHandler(store Store) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(store, "test-token", logger)
}

func TestIngestRejectsMissingToken(t *testing.T) {
	store := &fakeStore{}
	request := httptest.NewRequest(http.MethodPost, "/api/ingest",
		bytes.NewBufferString(`{"source":"contractors","records":[{"company":"Example"}]}`))
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if len(store.ingestedContractors) != 0 {
		t.Fatal("store was called for an unauthorized request")
	}
}

func TestIngestContractors(t *testing.T) {
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{contractorRun: model.ScrapeRun{ID: 42, Source: model.SourceContractors, ScrapedAt: now}}
	request := httptest.NewRequest(http.MethodPost, "/api/ingest",
		bytes.NewBufferString(`{"source":"contractors","records":[{"id":7,"company":" Example Co ","prc":" REE "}]}`))
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if len(store.ingestedContractors) != 1 {
		t.Fatalf("ingested %d contractors, want 1", len(store.ingestedContractors))
	}
	if got := store.ingestedContractors[0].Company; got != "Example Co" {
		t.Fatalf("company = %q, want normalized value", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("POST CORS header = %q, want empty", got)
	}
}

func TestIngestRejectsEmptyRecords(t *testing.T) {
	store := &fakeStore{}
	request := httptest.NewRequest(http.MethodPost, "/api/ingest",
		bytes.NewBufferString(`{"source":"contractors","records":[]}`))
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestLatestSnapshotNotFoundAndCORS(t *testing.T) {
	store := &fakeStore{latestBarangayErr: db.ErrSnapshotNotFound}
	request := httptest.NewRequest(http.MethodGet, "/api/barangay-feeders/latest", nil)
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("GET CORS header = %q, want *", got)
	}
}

func TestHistoricalSnapshotRequiresPositiveRunID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/contractors?run_id=nope", nil)
	response := httptest.NewRecorder()

	testHandler(&fakeStore{}).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestHealthReportsDatabaseFailure(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	response := httptest.NewRecorder()

	testHandler(&fakeStore{healthErr: errors.New("database down")}).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
