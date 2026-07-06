package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(NewRedactingHandler(slog.NewJSONHandler(buf, nil)))
}

func TestRedactingHandler_SensitiveKey(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.Info("user login", "authorization", "Bearer abc123.def456.ghi789", "user_id", 42)

	out := buf.String()
	if strings.Contains(out, "abc123") {
		t.Fatalf("expected authorization value to be redacted, got: %s", out)
	}
	if !strings.Contains(out, redacted) {
		t.Fatalf("expected [REDACTED] marker in output, got: %s", out)
	}
	if !strings.Contains(out, `"user_id":42`) {
		t.Fatalf("expected non-sensitive field to pass through untouched, got: %s", out)
	}
}

func TestRedactingHandler_PatternInString(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.Error("upstream call failed", "detail", "request had header Bearer sk-ant-abcDEF123 attached")

	out := buf.String()
	if strings.Contains(out, "sk-ant-abcDEF123") {
		t.Fatalf("expected embedded secret to be redacted, got: %s", out)
	}
	if !strings.Contains(out, redacted) {
		t.Fatalf("expected [REDACTED] marker in output, got: %s", out)
	}
}

func TestRedactingHandler_WrappedError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	err := fmt.Errorf("anthropic request failed: %w", errors.New("dial tcp: header x-api-key: sk-ant-secretvalue"))
	logger.Error("scan failed", "error", err)

	out := buf.String()
	if strings.Contains(out, "sk-ant-secretvalue") {
		t.Fatalf("expected secret embedded in wrapped error to be redacted, got: %s", out)
	}
}

func TestRedactingHandler_WithAttrsRedacts(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(&buf)
	logger := base.With("token", "super-secret-value")

	logger.Info("request handled")

	out := buf.String()
	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("expected attribute bound via With() to be redacted, got: %s", out)
	}
}

func TestRedactingHandler_PassesThroughOrdinaryMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.Info("group created", "group_id", 7, "name", "Trip to BA")

	out := buf.String()
	if !strings.Contains(out, "Trip to BA") || !strings.Contains(out, `"group_id":7`) {
		t.Fatalf("expected non-sensitive fields to pass through untouched, got: %s", out)
	}
}
