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

// internalError logs the real error server-side and returns a generic 500 to
// the client. A 500 means something on our side went wrong (a DB error, a
// bug) — the raw message can contain internal details (query fragments,
// driver errors) that have no business being handed to whoever made the
// request, so it never goes in the response body.
func internalError(w http.ResponseWriter, logMsg string, err error) {
	slog.Error(logMsg, "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
