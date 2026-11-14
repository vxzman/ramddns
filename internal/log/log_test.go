package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSanitizeMessage(t *testing.T) {
	in := `token=abcdefghijklmnopqrstuvwxyz secret: "0123456789abcdef"`
	got := sanitizeMessage(in)
	if strings.Contains(got, "abcdefghijklmnopqrstuvwxyz") || strings.Contains(got, "0123456789abcdef") {
		t.Fatalf("secret leaked: %s", got)
	}
	if !strings.Contains(got, "token=") || !strings.Contains(got, "***REDACTED***") {
		t.Fatalf("redaction = %s", got)
	}
}

func TestRedactHandler(t *testing.T) {
	var buf bytes.Buffer
	h := redactHandler{next: slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "cloudflare", 0)
	record.Add("token", "abcdefghijklmnopqrstuvwxyz", "zone", "example.com")
	if err := h.Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("token leaked: %s", out)
	}
	if !strings.Contains(out, "***REDACTED***") || !strings.Contains(out, "example.com") {
		t.Fatalf("log = %s", out)
	}
}
