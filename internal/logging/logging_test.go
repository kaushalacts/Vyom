package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func newTestLogger(t *testing.T, cfg Config) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	cfg.Writer = buf
	if cfg.Format == "" {
		cfg.Format = FormatJSON
	}
	return New(cfg), buf
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log output produced")
	}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	return m
}

func TestNewEmitsJSONWithServiceAttribute(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{Level: LevelInfo})
	logger.Info("scan complete", "assets", 182431)

	m := decode(t, buf)
	if m["msg"] != "scan complete" {
		t.Errorf("msg = %v", m["msg"])
	}
	if m["service"] != "vyom" {
		t.Errorf("service = %v, want vyom", m["service"])
	}
	if m["assets"] != float64(182431) {
		t.Errorf("assets = %v", m["assets"])
	}
}

func TestLevelFiltering(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{Level: LevelWarn})
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	out := buf.String()
	if strings.Contains(out, "debug message") || strings.Contains(out, "info message") {
		t.Errorf("level filter failed:\n%s", out)
	}
	if !strings.Contains(out, "warn message") || !strings.Contains(out, "error message") {
		t.Errorf("expected warn and error:\n%s", out)
	}
}

func TestRedactorHidesSensitiveAttributeKeys(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{Redactor: NewRedactor()})
	logger.Info("connecting", "client_secret", "shhh-dont-tell", "host", "akamai.example.net")

	out := buf.String()
	if strings.Contains(out, "shhh-dont-tell") {
		t.Fatalf("secret attribute leaked:\n%s", out)
	}
	if !strings.Contains(out, "akamai.example.net") {
		t.Errorf("non-secret attribute was dropped:\n%s", out)
	}
	m := decode(t, buf)
	if m["client_secret"] != DefaultPlaceholder {
		t.Errorf("client_secret = %v, want %q", m["client_secret"], DefaultPlaceholder)
	}
}

func TestRedactorHidesLiteralSecretValuesInMessageAndValues(t *testing.T) {
	t.Parallel()
	const secret = "super-secret-token-value-9f8a7b6c"
	logger, buf := newTestLogger(t, Config{Redactor: NewRedactor(secret)})
	logger.Info("using token "+secret, "access_token", secret)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("literal secret leaked:\n%s", out)
	}
	if !strings.Contains(out, DefaultPlaceholder) {
		t.Errorf("expected placeholder:\n%s", out)
	}
}

func TestRedactorHidesSecretsInsideGroups(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{Redactor: NewRedactor()})
	logger.WithGroup("provider").With("client_secret", "nested-secret-value").Info("connect")

	out := buf.String()
	if strings.Contains(out, "nested-secret-value") {
		t.Fatalf("secret inside a group leaked:\n%s", out)
	}
}

func TestRedactorHidesSecretsInWithAttrs(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{Redactor: NewRedactor()})
	// The redaction must also apply to attributes attached via With, not only
	// to those passed to a log call.
	logger.With("client_secret", "attached-secret-value").Info("hello")

	if strings.Contains(buf.String(), "attached-secret-value") {
		t.Fatalf("secret from With() leaked:\n%s", buf.String())
	}
}

func TestRedactorScrubsPresignedURLs(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{})
	// An Azure SAS URL: the ?se=, &sig= and &sp= parameters are credentials.
	logger.Info("download failed",
		"url", "https://acct.blob.core.windows.net/c/a.jpg?sv=2021-08-06&se=2026-09-01&sp=r&sig=ABCDEF123456%3D")

	out := buf.String()
	for _, forbidden := range []string{"ABCDEF123456", "se=2026-09-01", "sp=r"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("presigned credential %q leaked:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(out, "a.jpg") {
		t.Errorf("redaction removed useful context:\n%s", out)
	}
	if !strings.Contains(out, "sv=2021-08-06") {
		t.Errorf("non-secret query param should survive:\n%s", out)
	}
}

func TestRedactorIsOptional(t *testing.T) {
	t.Parallel()
	// Even with no redactor, presigned URL credentials must never be logged.
	logger, buf := newTestLogger(t, Config{})
	logger.Info("x", "url", "https://h/c/o?X-Amz-Signature=deadbeefcafe&X-Amz-Credential=AKIA/key")
	if strings.Contains(buf.String(), "deadbeefcafe") || strings.Contains(buf.String(), "AKIA/key") {
		t.Errorf("credentials leaked without a redactor:\n%s", buf.String())
	}
}

func TestTextFormat(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	New(Config{Writer: buf, Format: FormatText}).Info("hello", "n", 1)
	if !strings.Contains(buf.String(), "level=INFO") || !strings.Contains(buf.String(), "msg=hello") {
		t.Errorf("unexpected text output: %q", buf.String())
	}
}

func TestComponentAttribute(t *testing.T) {
	t.Parallel()
	logger, buf := newTestLogger(t, Config{Component: "scanner"})
	logger.Info("fetching")
	if m := decode(t, buf); m["component"] != "scanner" {
		t.Errorf("component = %v", m["component"])
	}
}

func TestParseLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    Level
		wantErr bool
	}{
		{"debug", LevelDebug, false},
		{"INFO", LevelInfo, false},
		{"warn", LevelWarn, false},
		{"warning", LevelWarn, false},
		{"", LevelInfo, false},
		{"error", LevelError, false},
		{"verbose", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseLevel(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseLevel(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]Format{
		"": FormatJSON, "json": FormatJSON, "text": FormatText, "CONSOLE": FormatText,
	} {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("expected an error for an unknown format")
	}
}

func TestNewNopWritesNothing(t *testing.T) {
	t.Parallel()
	// NewNop must not panic and must accept calls; it is used by tests.
	NewNop().Info("ignored", "k", "v")
}

func TestSensitiveKeyDetection(t *testing.T) {
	t.Parallel()
	r := NewRedactor()
	for _, k := range []string{"secret", "CLIENT_SECRET", "AccessToken", "api-key", "password"} {
		if !r.IsSensitiveKey(k) {
			t.Errorf("%q should be treated as sensitive", k)
		}
	}
	for _, k := range []string{"host", "container", "requests"} {
		if r.IsSensitiveKey(k) {
			t.Errorf("%q must not be treated as sensitive", k)
		}
	}
}
