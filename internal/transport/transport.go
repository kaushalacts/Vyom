// Package transport holds the small shared interfaces that provider
// constructors need.
//
// It exists to break an import cycle: both internal/cdn and internal/storage
// need to accept an HTTP client and a logger without either importing the
// other or importing the provider registry. The interfaces are declared here,
// in a leaf package, and satisfied by *http.Client and *slog.Logger
// respectively.
package transport

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Doer is the subset of *http.Client that Vyom's providers use.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Logger is the subset of *slog.Logger that Vyom's providers use.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// ClientOptions configures the shared HTTP client used by every provider.
type ClientOptions struct {
	// Timeout bounds a single request including the response body read.
	Timeout time.Duration
	// MaxIdleConnsPerHost controls connection reuse. Provider APIs benefit
	// heavily from a high value when scanning with a worker pool.
	MaxIdleConnsPerHost int
	// MaxConnsPerHost bounds concurrent connections, which is Vyom's way of
	// self-throttling below a provider's documented rate limit.
	MaxConnsPerHost int
	// UserAgent identifies Vyom to providers.
	UserAgent string
}

// DefaultClientOptions returns conservative defaults.
func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		Timeout:             60 * time.Second,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     10,
		UserAgent:           "vyom",
	}
}

// NewClient builds an *http.Client from opts. A zero Config yields a client
// with sensible defaults, so callers can pass what they have.
func NewClient(opts ClientOptions) *http.Client {
	def := DefaultClientOptions()
	if opts.Timeout <= 0 {
		opts.Timeout = def.Timeout
	}
	if opts.MaxIdleConnsPerHost <= 0 {
		opts.MaxIdleConnsPerHost = def.MaxIdleConnsPerHost
	}
	if opts.UserAgent == "" {
		opts.UserAgent = def.UserAgent
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		MaxConnsPerHost:       opts.MaxConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Timeout: opts.Timeout, Transport: transport}
}

// NopLogger returns a Logger that discards everything, for tests.
func NopLogger() Logger { return nopLogger{} }

type nopLogger struct{}

func (nopLogger) Debug(string, ...any) {}
func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
func (nopLogger) With(...any) Logger   { return nopLogger{} }

// Slog adapts a *slog.Logger to Logger. A nil logger yields a NopLogger so
// that providers never need a nil check.
func Slog(l *slog.Logger) Logger {
	if l == nil {
		return NopLogger()
	}
	return slogAdapter{l}
}

type slogAdapter struct{ l *slog.Logger }

func (s slogAdapter) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s slogAdapter) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s slogAdapter) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s slogAdapter) Error(msg string, args ...any) { s.l.Error(msg, args...) }
func (s slogAdapter) With(args ...any) Logger       { return slogAdapter{s.l.With(args...)} }

// DoerFunc adapts a function to Doer.
type DoerFunc func(*http.Request) (*http.Response, error)

// Do implements Doer.
func (f DoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// contextKey is the private key type for context values set by Vyom.
type contextKey int

const requestIDKey contextKey = iota

// WithRequestID returns a context carrying a request identifier, so that
// provider logs can be correlated with API requests and scan jobs.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the request identifier stored in ctx, or "".
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}
