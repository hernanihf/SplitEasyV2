package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestLogger replaces chi's middleware.Logger, which logs r.RequestURI —
// the path *and* query string — via its own unstructured stdlib logger,
// bypassing both the JSON logging the rest of the app uses and the
// RedactingHandler safety net wrapped around it (that only sees calls made
// through slog). GET /groups/preview?token=... would otherwise put the
// invite token in the access log on every request. Logging r.URL.Path only
// closes that off for this and any future query-string parameter, not just
// this one token.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		defer func() {
			//nolint:gosec // G706: JSON handler encodes values, can't inject log lines
			slog.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start).String(),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
