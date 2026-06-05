package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Envelope map[string]any

func WithJson(w http.ResponseWriter, statusCode int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("respond: marshal failed", "err", err, "status", statusCode)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		slog.Warn("respond: write failed", "err", err)
	}
}

func WithError(w http.ResponseWriter, statusCode int, message string) {
	writeError(w, statusCode, message)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	data, err := json.Marshal(Envelope{"error": message})
	if err != nil {
		slog.Error("respond: marshal error envelope failed", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		slog.Warn("respond: write error failed", "err", err)
	}
}
