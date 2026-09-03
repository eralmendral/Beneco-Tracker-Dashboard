package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return fmt.Errorf("request body must not exceed %d bytes", maxRequestBytes)
		}
		return errors.New("request body must be valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func validBearerToken(header, expected string) bool {
	const prefix = "Bearer "
	if expected == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	actual := strings.TrimPrefix(header, prefix)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func readRunID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.URL.Query().Get("run_id")
	runID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || runID <= 0 {
		writeError(w, http.StatusBadRequest, "run_id must be a positive integer")
		return 0, false
	}
	return runID, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handler) internalError(w http.ResponseWriter, operation string, err error) {
	h.logger.Error(operation, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
