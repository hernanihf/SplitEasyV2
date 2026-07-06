package config

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// sensitiveKeys are slog attribute keys whose value is replaced outright,
// regardless of content — covers a secret logged by full value under a
// name that gives it away (e.g. slog.String("authorization", header)).
var sensitiveKeys = []string{
	"password", "secret", "token", "authorization", "api_key", "apikey", "jwt",
}

// secretPatterns catch a secret embedded inside a larger string, where the
// key name alone gives no hint (e.g. an error message that happens to
// include a header value). Matches are replaced in place so the rest of the
// message stays readable.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+\S+`),                                      // "Bearer <token>"
	regexp.MustCompile(`(?i)x-api-key:\s*\S+`),                                  // dumped header line
	regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\b`), // JWT
	regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9_-]+\b`),                             // Anthropic key
}

// RedactingHandler wraps another slog.Handler, scrubbing attribute values
// that look like secrets before they reach it (and, from there, stdout/log
// aggregation). This is a safety net for future logging mistakes — every
// current call site already avoids logging secrets directly.
type RedactingHandler struct {
	slog.Handler
}

func NewRedactingHandler(inner slog.Handler) *RedactingHandler {
	return &RedactingHandler{Handler: inner}
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, redactString(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(redactAttr(a))
		return true
	})
	return h.Handler.Handle(ctx, nr)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = redactAttr(a)
	}
	return &RedactingHandler{Handler: h.Handler.WithAttrs(scrubbed)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{Handler: h.Handler.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, k := range sensitiveKeys {
		if strings.Contains(key, k) {
			return slog.String(a.Key, redacted)
		}
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, redactString(a.Value.String()))
	}
	// Errors are stored as KindAny — stringify so a pattern embedded in the
	// message (e.g. a wrapped HTTP error mentioning a header) is still caught.
	if err, ok := a.Value.Any().(error); ok {
		return slog.String(a.Key, redactString(err.Error()))
	}
	return a
}

func redactString(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, redacted)
	}
	return s
}
