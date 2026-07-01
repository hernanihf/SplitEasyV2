package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeJSON writes v as the JSON response body with the given status code.
// Encode can still fail (e.g. the client disconnects mid-write); by then the
// status/headers are already sent so the response itself can't be changed,
// but the failure is logged instead of disappearing silently.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
