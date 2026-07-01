package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer recovers from panics in downstream handlers and returns a
// generic 500 with no body, same as chi's middleware.Recoverer — but logs
// through slog instead. chi's version writes straight to os.Stderr as
// unstructured text (PrintPrettyStack), bypassing the JSON logging the rest
// of the app uses, so a panic wouldn't show up structured in whatever's
// aggregating production logs.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				if rvr == http.ErrAbortHandler { //nolint:errorlint // sentinel value, not a wrapped error
					// Don't recover this one — it means the handler wants the
					// response aborted without being treated as an error.
					panic(rvr)
				}

				// gosec flags r.Method/r.URL.Path as unsanitized input reaching
				// a log sink (G706). The production logger is always
				// slog.NewJSONHandler (set once in main()), which encodes each
				// field as a JSON string value — there's no way for a crafted
				// path to break out and forge a separate log entry.
				slog.Error("panic recovered", //nolint:gosec // G706: JSON handler encodes values, can't inject log lines
					"panic", rvr,
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
				)

				if r.Header.Get("Connection") != "Upgrade" {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}
		}()

		next.ServeHTTP(w, r)
	})
}
