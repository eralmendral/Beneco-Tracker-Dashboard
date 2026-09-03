package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/db"
	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/model"
)

const (
	maxRequestBytes = 10 << 20
	maxRecords      = 10_000
)

type Store interface {
	Health(ctx context.Context) error
	IngestBarangayFeeders(ctx context.Context, records []model.BarangayFeeder) (model.ScrapeRun, error)
	IngestContractors(ctx context.Context, records []model.Contractor) (model.ScrapeRun, error)
	ListScrapeRuns(ctx context.Context, source string) ([]model.ScrapeRun, error)
	LatestBarangayFeeders(ctx context.Context) ([]model.BarangayFeeder, error)
	BarangayFeedersByRun(ctx context.Context, runID int64) ([]model.BarangayFeeder, error)
	LatestContractors(ctx context.Context) ([]model.Contractor, error)
	ContractorsByRun(ctx context.Context, runID int64) ([]model.Contractor, error)
}

type Handler struct {
	store       Store
	ingestToken string
	logger      *slog.Logger
}

type ingestRequest struct {
	Source  string          `json:"source"`
	Records json.RawMessage `json:"records"`
}

type ingestResponse struct {
	RunID       int64     `json:"run_id"`
	Source      string    `json:"source"`
	ScrapedAt   time.Time `json:"scraped_at"`
	RecordCount int       `json:"record_count"`
}

func NewHandler(store Store, ingestToken string, logger *slog.Logger) http.Handler {
	h := &Handler{store: store, ingestToken: ingestToken, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", h.health)
	mux.HandleFunc("POST /api/ingest", h.ingest)
	mux.HandleFunc("GET /api/scrape-runs", h.scrapeRuns)
	mux.HandleFunc("GET /api/barangay-feeders/latest", h.latestBarangayFeeders)
	mux.HandleFunc("GET /api/barangay-feeders", h.barangayFeedersByRun)
	mux.HandleFunc("GET /api/contractors/latest", h.latestContractors)
	mux.HandleFunc("GET /api/contractors", h.contractorsByRun)
	return h.middleware(mux)
}

func (h *Handler) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		h.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.store.Health(ctx); err != nil {
		h.logger.Error("database health check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	if !validBearerToken(r.Header.Get("Authorization"), h.ingestToken) {
		writeError(w, http.StatusUnauthorized, "invalid bearer token")
		return
	}

	var request ingestRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch request.Source {
	case model.SourceBarangayFeeders:
		var records []model.BarangayFeeder
		if err := json.Unmarshal(request.Records, &records); err != nil {
			writeError(w, http.StatusBadRequest, "records must be a barangay feeder array")
			return
		}
		if err := validateBarangayFeeders(records); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := h.store.IngestBarangayFeeders(r.Context(), records)
		if err != nil {
			h.internalError(w, "ingest barangay feeders", err)
			return
		}
		writeJSON(w, http.StatusCreated, ingestResponse{run.ID, run.Source, run.ScrapedAt, len(records)})
	case model.SourceContractors:
		var records []model.Contractor
		if err := json.Unmarshal(request.Records, &records); err != nil {
			writeError(w, http.StatusBadRequest, "records must be a contractor array")
			return
		}
		if err := validateContractors(records); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := h.store.IngestContractors(r.Context(), records)
		if err != nil {
			h.internalError(w, "ingest contractors", err)
			return
		}
		writeJSON(w, http.StatusCreated, ingestResponse{run.ID, run.Source, run.ScrapedAt, len(records)})
	default:
		writeError(w, http.StatusBadRequest, "source must be barangay_feeders or contractors")
	}
}

func (h *Handler) scrapeRuns(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source != "" && source != model.SourceBarangayFeeders && source != model.SourceContractors {
		writeError(w, http.StatusBadRequest, "source must be barangay_feeders or contractors")
		return
	}
	runs, err := h.store.ListScrapeRuns(r.Context(), source)
	if err != nil {
		h.internalError(w, "list scrape runs", err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *Handler) latestBarangayFeeders(w http.ResponseWriter, r *http.Request) {
	records, err := h.store.LatestBarangayFeeders(r.Context())
	h.writeSnapshot(w, records, err)
}

func (h *Handler) barangayFeedersByRun(w http.ResponseWriter, r *http.Request) {
	runID, ok := readRunID(w, r)
	if !ok {
		return
	}
	records, err := h.store.BarangayFeedersByRun(r.Context(), runID)
	h.writeSnapshot(w, records, err)
}

func (h *Handler) latestContractors(w http.ResponseWriter, r *http.Request) {
	records, err := h.store.LatestContractors(r.Context())
	h.writeSnapshot(w, records, err)
}

func (h *Handler) contractorsByRun(w http.ResponseWriter, r *http.Request) {
	runID, ok := readRunID(w, r)
	if !ok {
		return
	}
	records, err := h.store.ContractorsByRun(r.Context(), runID)
	h.writeSnapshot(w, records, err)
}

func (h *Handler) writeSnapshot(w http.ResponseWriter, records any, err error) {
	if errors.Is(err, db.ErrSnapshotNotFound) {
		writeError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	if err != nil {
		h.internalError(w, "read snapshot", err)
		return
	}
	writeJSON(w, http.StatusOK, records)
}
