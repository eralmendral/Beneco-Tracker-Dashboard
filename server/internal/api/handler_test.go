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
	barangayRun         model.ScrapeRun
	contractorRun       model.ScrapeRun
	outageRun           model.ScrapeRun
	facebookRun         model.ScrapeRun
	ingestedBarangays   []model.BarangayFeeder
	ingestedContractors []model.Contractor
	ingestedOutages     []model.OutageEvent
	ingestedFacebook    []model.FacebookReport
	latestBarangays     []model.BarangayFeeder
	latestBarangayErr   error
	latestContractors   []model.Contractor
	latestContractorErr error
	outages             []model.OutageEvent
	facebookReports     []model.FacebookReport
}

func (f *fakeStore) Health(context.Context) error { return f.healthErr }

func (f *fakeStore) IngestBarangayFeeders(_ context.Context, records []model.BarangayFeeder) (model.ScrapeRun, error) {
	f.ingestedBarangays = records
	return f.barangayRun, nil
}

func (f *fakeStore) IngestContractors(_ context.Context, records []model.Contractor) (model.ScrapeRun, error) {
	f.ingestedContractors = records
	return f.contractorRun, nil
}

func (f *fakeStore) IngestOutages(_ context.Context, records []model.OutageEvent) (model.ScrapeRun, error) {
	f.ingestedOutages = records
	return f.outageRun, nil
}

func (f *fakeStore) IngestFacebookReports(_ context.Context, records []model.FacebookReport) (model.ScrapeRun, error) {
	f.ingestedFacebook = records
	return f.facebookRun, nil
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
	return f.latestContractors, f.latestContractorErr
}

func (f *fakeStore) ContractorsByRun(context.Context, int64) ([]model.Contractor, error) {
	return []model.Contractor{}, nil
}

func (f *fakeStore) OutagesSince(context.Context, time.Time) ([]model.OutageEvent, error) {
	return f.outages, nil
}

func (f *fakeStore) FacebookReportsSince(context.Context, time.Time) ([]model.FacebookReport, error) {
	return f.facebookReports, nil
}

type fakeScraper struct {
	err error
}

func (f *fakeScraper) Run(context.Context) error { return f.err }

func testHandler(store Store) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(store, "test-token", logger, &fakeScraper{})
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

func TestIngestOutages(t *testing.T) {
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{outageRun: model.ScrapeRun{ID: 43, Source: model.SourceOutages, ScrapedAt: now}}
	request := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewBufferString(
		`{"source":"outages","records":[{"source_id":"68012:FEEDER_12","feeder":" FEEDER_12 ","area":" Woodsgate ","started_at":"2026-09-02T03:37:03Z","duration_minutes":419,"status":"Restored","source_url":"https://beneco.com.ph/"}]}`))
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if len(store.ingestedOutages) != 1 || store.ingestedOutages[0].Feeder != "FEEDER_12" {
		t.Fatalf("ingested outages = %#v", store.ingestedOutages)
	}
}

func TestIngestFacebookComplaintExcerpt(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{facebookRun: model.ScrapeRun{ID: 44, Source: model.SourceFacebookReports, ScrapedAt: now}}
	request := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewBufferString(
		`{"source":"facebook_reports","records":[{"source_id":"comment-1","post_url":"https://www.facebook.com/benguetelectric/posts/123/","reported_at":"2026-09-03T02:00:00Z","location":" Camp 7 ","feeder":" FEEDER_12 ","comment_excerpt":" No power since this morning "}]}`))
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if len(store.ingestedFacebook) != 1 || store.ingestedFacebook[0].CommentExcerpt != "No power since this morning" {
		t.Fatalf("ingested Facebook reports = %#v", store.ingestedFacebook)
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

func TestScrapeRejectsMissingToken(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/scrape", nil)
	response := httptest.NewRecorder()

	testHandler(&fakeStore{}).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestScrapeReturnsLatestRecordCounts(t *testing.T) {
	store := &fakeStore{
		latestBarangays:   []model.BarangayFeeder{{BarangayID: 1}, {BarangayID: 2}},
		latestContractors: []model.Contractor{{Company: "Example"}},
		outages:           []model.OutageEvent{{SourceID: "1:FEEDER_08"}},
		facebookReports:   []model.FacebookReport{{SourceID: "report:FEEDER_12"}},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/scrape", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	testHandler(store).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got := response.Body.String(); got != "{\"status\":\"ok\",\"barangay_feeders\":2,\"contractors\":1,\"outages\":1,\"facebook_reports\":1}\n" {
		t.Fatalf("body = %s", got)
	}
}

func TestOutagesRejectsInvalidWindow(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/outages?days=2", nil)
	response := httptest.NewRecorder()

	testHandler(&fakeStore{}).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestScrapeReportsRunnerFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(&fakeStore{}, "test-token", logger, &fakeScraper{err: errors.New("upstream unavailable")})
	request := httptest.NewRequest(http.MethodPost, "/api/scrape", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
}
