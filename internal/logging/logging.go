// Package logging builds Vyom's structured loggers.
//
// Vyom uses log/slog from the standard library: JSON in production, human
// readable text in a terminal. On top of that it enforces two properties that
// matter for an infrastructure tool:
//
//  1. Every secret Vyom knows about is redacted before it can reach a log line.
//  2. Every log line carries a scan or request identifier once one is known,
//     so that a single user-visible operation can be reconstructed from logs.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// Level is Vyom's log level vocabulary.
type Level = slog.Level

// Log levels.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Format selects the output encoding.
type Format string

// Output formats.
const (
	// FormatJSON is one JSON object per line: for log shippers and jq.
	FormatJSON Format = "json"
	// FormatText is human readable: for a terminal.
	FormatText Format = "text"
)

// Config configures a logger.
type Config struct {
	// Level is one of debug, info, warn, error. Default: info.
	Level Level
	// Format is json or text. Default: json.
	Format Format
	// Writer is the destination. Default: os.Stderr.
	Writer io.Writer
	// Service is added as a constant attribute (defaults to "vyom").
	Service string
	// Component identifies the emitting subsystem, for example "scanner".
	Component string
	// Redactor scrubs known secrets. A nil redactor still removes
	// credential-shaped URL parameters.
	Redactor *Redactor
	// AddSource includes file:line. Useful in development, noisy in production.
	AddSource bool
}

// Redactor scrubs secrets and credential-bearing values from log output.
//
// It wraps an slog.Handler so that redaction cannot be bypassed by accident:
// attributes and message text are both filtered, regardless of whether a caller
// remembered to redact a value itself.
type Redactor struct {
	// values are literal secrets known at runtime (tokens, client secrets).
	values []string
	// sensitiveKeys are attribute names whose value is always replaced.
	sensitiveKeys map[string]bool
	placeholder   string
}

// DefaultPlaceholder replaces redacted values.
const DefaultPlaceholder = "[REDACTED]"

// sensitiveKeyNames are attribute keys whose values are always redacted,
// regardless of their content. The list is intentionally broad: over-redacting
// a debug field is free, leaking a credential is not.
var sensitiveKeyNames = []string{
	"secret", "client_secret", "clientsecret", "token", "access_token", "accesstoken",
	"refresh_token", "password", "passwd", "authorization", "credential",
	"credentials", "api_key", "apikey", "api-key", "account_key", "accountkey",
	"subscription_key", "signature", "private_key", "privatekey", "sas_token",
	"session_token", "bearer", "x-amz-signature", "x-amz-credential", "sig",
}

// NewRedactor builds a Redactor for literal secret values.
func NewRedactor(values ...string) *Redactor {
	r := &Redactor{
		sensitiveKeys: make(map[string]bool, len(sensitiveKeyNames)),
		placeholder:   DefaultPlaceholder,
	}
	for _, k := range sensitiveKeyNames {
		r.sensitiveKeys[k] = true
	}
	for _, v := range values {
		v = strings.TrimSpace(v)
		// Very short values would match unrelated text and mangle output.
		if len(v) < 8 {
			continue
		}
		r.values = append(r.values, v)
	}
	return r
}

// IsSensitiveKey reports whether an attribute name is credential-shaped.
func (r *Redactor) IsSensitiveKey(key string) bool {
	if r == nil {
		return false
	}
	return r.sensitiveKeys[strings.ToLower(strings.TrimSpace(key))]
}

// Text redacts a string: known literal secrets plus credential-shaped URL
// query parameters (pre-signed URLs must never appear verbatim in a log).
func (r *Redactor) Text(s string) string {
	if r == nil {
		return redactQueryParams(s)
	}
	for _, v := range r.values {
		s = strings.ReplaceAll(s, v, r.placeholder)
	}
	return redactQueryParams(s)
}

func (r *Redactor) placeholderOr() string {
	if r == nil || r.placeholder == "" {
		return DefaultPlaceholder
	}
	return r.placeholder
}

// handler wraps an slog.Handler with redaction.
type handler struct {
	inner slog.Handler
	red   *Redactor
	// attrs holds attributes added at construction time.
	attrs []slog.Attr
	group string
}

// New builds a logger from cfg.
func New(cfg Config) *slog.Logger {
	if cfg.Writer == nil {
		cfg.Writer = defaultWriter()
	}
	if cfg.Service == "" {
		cfg.Service = "vyom"
	}
	if cfg.Format == "" {
		cfg.Format = FormatJSON
	}

	level := cfg.Level
	opts := &slog.HandlerOptions{Level: level, AddSource: cfg.AddSource}

	var inner slog.Handler
	switch cfg.Format {
	case FormatText:
		inner = slog.NewTextHandler(cfg.Writer, opts)
	default:
		inner = slog.NewJSONHandler(cfg.Writer, opts)
	}

	base := slog.New(handler{inner: inner, red: cfg.Redactor})
	logger := base.With(slog.String("service", cfg.Service))
	if cfg.Component != "" {
		logger = logger.With(slog.String("component", cfg.Component))
	}
	return logger
}

// NewNop returns a logger that discards everything, for tests.
func NewNop() *slog.Logger {
	return slog.New(handler{inner: slog.NewTextHandler(io.Discard, nil)})
}

// Handle redacts the record before delegating.
func (h handler) Handle(ctx context.Context, rec slog.Record) error {
	if h.red != nil {
		rec.Message = h.red.Text(rec.Message)
	}
	// Copy the record: mutating the caller's record is not allowed and the
	// attribute slice may be shared.
	out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	out.AddAttrs(h.attrs...)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.attr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

// attr redacts a single attribute, flattening groups so that nested secrets
// cannot hide.
func (h handler) attr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		if len(group) == 0 {
			return slog.Attr{}
		}
		// Redact recursively: a secret must not survive by hiding one
		// group deeper, e.g. logger.With(...).WithGroup("creds").
		safe := make([]slog.Attr, 0, len(group))
		for _, g := range group {
			if ga := h.attr(g); !ga.Equal(slog.Attr{}) {
				safe = append(safe, ga)
			}
		}
		if len(safe) == 0 {
			return slog.Attr{}
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(safe...)}
	}
	if h.red.IsSensitiveKey(a.Key) {
		return slog.String(a.Key, h.red.placeholderOr())
	}
	// Redactor.Text is nil-safe and always strips credential-bearing URL
	// parameters, so a string attribute is filtered even when no runtime
	// secret list was supplied.
	if s, ok := a.Value.Any().(string); ok {
		if red := h.red.Text(s); red != s {
			return slog.String(a.Key, red)
		}
	}
	return a
}

func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if sa := h.attr(a); !sa.Equal(slog.Attr{}) {
			redacted = append(redacted, sa)
		}
	}
	return handler{inner: h.inner.WithAttrs(redacted), red: h.red, group: h.group}
}

func (h handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return handler{inner: h.inner.WithGroup(name), red: h.red, group: name}
}

// Enabled implements slog.Handler.
func (h handler) Enabled(_ context.Context, level slog.Level) bool {
	return h.inner.Enabled(context.Background(), level)
}

// ParseLevel converts a user-supplied string to a Level.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "", "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level %q (want debug, info, warn or error)", s)
	}
}

// ParseFormat converts a user-supplied string to a Format.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "json":
		return FormatJSON, nil
	case "text", "console", "plain":
		return FormatText, nil
	default:
		return FormatJSON, fmt.Errorf("unknown log format %q (want json or text)", s)
	}
}

// LevelNames lists the supported levels, for help text and validation messages.
func LevelNames() []string { return []string{"debug", "info", "warn", "error"} }

var (
	defaultWriterOnce sync.Once
	defaultWriterVal  io.Writer
)

func defaultWriter() io.Writer {
	defaultWriterOnce.Do(func() { defaultWriterVal = stderr() })
	return defaultWriterVal
}
