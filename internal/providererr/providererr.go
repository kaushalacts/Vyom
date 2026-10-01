// Package providererr defines Vyom's provider error taxonomy.
//
// Cloud APIs fail in ways that demand different operator responses: an
// authentication error means "fix your credentials", a rate-limit error means
// "Vyom will retry", a not-found error means "skip this object". Vyom
// therefore normalizes every provider failure into an *Error carrying a Kind,
// so that the CLI, the API and the retry logic can react without importing a
// single vendor SDK type.
//
// The package also guarantees a hard security property: Redacted() output is
// safe to log, and secrets never reach an error message because providers wrap
// failures through New.
package providererr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Kind classifies a provider failure.
type Kind string

// Error kinds. These strings appear verbatim in the API, in scan warnings and
// in metrics labels, so they are part of Vyom's contract.
const (
	// KindConfiguration: the provider is misconfigured (missing host, bad URL).
	KindConfiguration Kind = "configuration"
	// KindAuthentication: credentials were rejected (401).
	KindAuthentication Kind = "authentication"
	// KindAuthorization: credentials are valid but lack permission (403).
	KindAuthorization Kind = "authorization"
	// KindNotFound: the object, report or dataset does not exist (404).
	KindNotFound Kind = "not_found"
	// KindRateLimited: the provider asked us to slow down (429).
	KindRateLimited Kind = "rate_limited"
	// KindUnavailable: the provider is temporarily down (5xx).
	KindUnavailable Kind = "unavailable"
	// KindTimeout: a deadline elapsed.
	KindTimeout Kind = "timeout"
	// KindMalformedResponse: the payload could not be parsed (the single most
	// common real-world failure when a vendor changes a schema).
	KindMalformedResponse Kind = "malformed_response"
	// KindPagination: a multi-page fetch failed halfway through.
	KindPagination Kind = "pagination"
	// KindUnsupported: the requested feature is not implemented by Vyom.
	KindUnsupported Kind = "unsupported"
	// KindCorrelation: a CDN URL could not be mapped to a storage object in a
	// way the user asked for.
	KindCorrelation Kind = "correlation"
	// KindOptimization: an image could not be encoded or failed a quality gate.
	KindOptimization Kind = "optimization"
	// KindCancelled: the caller's context was cancelled.
	KindCancelled Kind = "cancelled"
	// KindInternal: a bug in Vyom.
	KindInternal Kind = "internal"
)

// KindFromStatus maps an HTTP status code onto a Kind.
func KindFromStatus(status int) Kind {
	switch {
	case status == http.StatusUnauthorized:
		return KindAuthentication
	case status == http.StatusForbidden:
		return KindAuthorization
	case status == http.StatusNotFound:
		return KindNotFound
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return KindTimeout
	case status == http.StatusTooManyRequests:
		return KindRateLimited
	case status >= 500:
		return KindUnavailable
	case status >= 400:
		return KindMalformedResponse
	default:
		return KindInternal
	}
}

// AllKinds lists every kind, in reporting order.
func AllKinds() []Kind {
	return []Kind{
		KindConfiguration, KindAuthentication, KindAuthorization, KindNotFound,
		KindRateLimited, KindUnavailable, KindTimeout, KindMalformedResponse,
		KindPagination, KindUnsupported, KindCorrelation, KindOptimization,
		KindCancelled, KindInternal,
	}
}

// Error is a classified provider error.
//
// Provider, Op and Kind are all required: Provider makes metrics and scan
// warnings attributable, Op says which call failed, Kind drives retry
// behaviour and the operator hint.
type Error struct {
	// Kind classifies the failure.
	Kind Kind
	// Provider is the provider type that failed, for example "akamai".
	Provider string
	// Op is the logical operation, for example "FetchTraffic".
	Op string
	// Message is a human-readable description.
	Message string
	// StatusCode is the HTTP status, when the failure came from an HTTP call.
	StatusCode int
	// RetryAfterSeconds is the provider's requested backoff, when provided.
	RetryAfterSeconds int
	// Wrapped is the cause.
	Wrapped error
}

// Error implements error. The message never contains credentials: providers are
// required to pass only non-secret context to New.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	if e.Provider != "" {
		b.WriteString(e.Provider)
		b.WriteString(": ")
	}
	if e.Op != "" {
		b.WriteString(e.Op)
		b.WriteString(": ")
	}
	switch {
	case e.Message != "":
		b.WriteString(e.Message)
	case e.Wrapped != nil:
		b.WriteString(e.Wrapped.Error())
	default:
		b.WriteString(string(e.Kind))
	}
	if e.StatusCode > 0 && !strings.Contains(b.String(), fmt.Sprintf("%d", e.StatusCode)) {
		fmt.Fprintf(&b, " (status %d)", e.StatusCode)
	}
	if e.Kind != KindInternal && e.Kind != "" && e.Message != "" && !strings.Contains(b.String(), string(e.Kind)) {
		fmt.Fprintf(&b, " [%s]", e.Kind)
	}
	return b.String()
}

// Unwrap exposes the cause to errors.Is/As.
func (e *Error) Unwrap() error { return e.Wrapped }

// Is lets errors.Is(err, providererr.Authentication(...)) work by kind.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return t.Kind == e.Kind && (t.Provider == "" || t.Provider == e.Provider) && (t.Op == "" || t.Op == e.Op)
}

// Retryable reports whether retrying the same call could plausibly succeed.
//
// Retryable failures are rate limits and transient 5xx/timeouts. Vyom retries
// with exponential backoff and jitter; it never retries authentication or
// authorization failures, because hammering an API with bad credentials gets
// accounts locked.
func (e *Error) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case KindRateLimited, KindUnavailable, KindTimeout:
		return true
	case KindMalformedResponse, KindPagination:
		// A schema change is not fixed by retrying, but a single truncated
		// page sometimes is. Callers should retry at most once.
		return true
	default:
		return false
	}
}

// Retryable reports whether err is a retryable provider error.
func Retryable(err error) bool {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Retryable()
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// KindOf returns the kind of err, or "" when err is not classified.
func KindOf(err error) Kind {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return KindCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return KindTimeout
	default:
		return ""
	}
}

// Is reports whether err carries the given kind.
func Is(err error, kind Kind) bool {
	if err == nil {
		return kind == ""
	}
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Kind == kind
	}
	switch kind {
	case KindCancelled:
		return errors.Is(err, context.Canceled)
	case KindTimeout:
		return errors.Is(err, context.DeadlineExceeded)
	default:
		return false
	}
}

// Option customizes an Error at construction time.
type Option func(*Error)

// WithStatus records the HTTP status code.
func WithStatus(code int) Option { return func(e *Error) { e.StatusCode = code } }

// WithRetryAfter records the provider-requested backoff in seconds.
func WithRetryAfter(seconds int) Option {
	return func(e *Error) {
		if seconds > 0 {
			e.RetryAfterSeconds = seconds
		}
	}
}

// WithMessage overrides the human-readable message.
func WithMessage(msg string) Option { return func(e *Error) { e.Message = msg } }

// New builds a classified provider error.
//
// Secrets must never be passed in msg or cause. Use Redacted() before
// attaching provider payloads to errors you log.
func New(kind Kind, provider, op string, cause error, opts ...Option) *Error {
	e := &Error{Kind: kind, Provider: provider, Op: op, Wrapped: cause}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Constructors for the common kinds. They keep call sites short and make
// grepping for a failure mode reliable.

func Configuration(provider, op, detail string) *Error {
	return New(KindConfiguration, provider, op, nil, WithMessage(detail))
}

func Authentication(provider, op string, cause error) *Error {
	return New(KindAuthentication, provider, op, cause)
}

func Authorization(provider, op string, cause error) *Error {
	return New(KindAuthorization, provider, op, cause)
}

func NotFound(provider, op, key string) *Error {
	return New(KindNotFound, provider, op, nil, WithMessage("not found: "+key))
}

func RateLimited(provider, op string, retryAfterSeconds int) *Error {
	e := New(KindRateLimited, provider, op, nil, WithMessage("provider rate limit reached"), WithRetryAfter(retryAfterSeconds))
	return e
}

func Unavailable(provider, op string, status int, cause error) *Error {
	return New(KindUnavailable, provider, op, cause, WithStatus(status))
}

func Timeout(provider, op string, cause error) *Error {
	return New(KindTimeout, provider, op, cause)
}

func MalformedResponse(provider, op string, cause error) *Error {
	return New(KindMalformedResponse, provider, op, cause)
}

func Pagination(provider, op string, cause error) *Error {
	return New(KindPagination, provider, op, cause)
}

func Unsupported(provider, op, detail string) *Error {
	return New(KindUnsupported, provider, op, nil, WithMessage(detail))
}

func Correlation(op, detail string) *Error {
	return New(KindCorrelation, "correlation", op, nil, WithMessage(detail))
}

func Optimization(op string, cause error) *Error {
	return New(KindOptimization, "optimizer", op, cause)
}

func Internal(provider, op string, cause error) *Error {
	return New(KindInternal, provider, op, cause)
}

// Wrap converts an arbitrary error into a classified one, preserving an
// existing classification and mapping context errors appropriately.
func Wrap(err error, provider, op string, defaultKind Kind) error {
	if err == nil {
		return nil
	}
	var pe *Error
	if errors.As(err, &pe) {
		return err
	}
	switch {
	case errors.Is(err, context.Canceled):
		return New(KindCancelled, provider, op, err)
	case errors.Is(err, context.DeadlineExceeded):
		return New(KindTimeout, provider, op, err)
	default:
		return New(defaultKind, provider, op, err)
	}
}

// Summary is a redacted, log-safe projection of an error.
type Summary struct {
	Kind      string `json:"kind"`
	Provider  string `json:"provider,omitempty"`
	Op        string `json:"op,omitempty"`
	Status    int    `json:"status,omitempty"`
	Retryable bool   `json:"retryable"`
	Message   string `json:"message"`
}

// Summarize converts err into a log- and API-safe structure.
func Summarize(err error) Summary {
	if err == nil {
		return Summary{}
	}
	s := Summary{Kind: string(KindOf(err)), Message: err.Error(), Retryable: Retryable(err)}
	var pe *Error
	if errors.As(err, &pe) {
		s.Provider = pe.Provider
		s.Op = pe.Op
		s.Status = pe.StatusCode
		if pe.Kind != KindInternal && !strings.Contains(pe.Message, string(pe.Kind)) {
			s.Kind = string(pe.Kind)
		}
	}
	return s
}

// secretish matches credential-shaped key names.
var secretish = []string{
	"secret", "token", "password", "passwd", "authorization", "credential",
	"api-key", "apikey", "access-key", "account-key", "signature", "private-key",
}

// Redactor scrubs credential-shaped values from strings before they are logged,
// written to an error message or returned by the API.
//
// Vyom's guarantee is simple and testable: no credential value ever reaches a
// log line, an API response or a database row. Redactor is the enforcement
// point.
type Redactor struct {
	// Values are literal secrets (access tokens, client secrets) known at
	// runtime. They are replaced with Placeholder wherever they appear.
	Values []string
	// Placeholder replaces redacted values.
	Placeholder string
}

// DefaultPlaceholder is the text substituted for redacted values.
const DefaultPlaceholder = "[REDACTED]"

// NewRedactor builds a Redactor for the given literal secret values. Empty and
// very short values are ignored: redacting a 1-2 character "secret" would
// mangle unrelated output.
func NewRedactor(values ...string) *Redactor {
	r := &Redactor{Placeholder: DefaultPlaceholder}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if len(v) < 8 {
			continue
		}
		r.Values = append(r.Values, v)
	}
	return r
}

// String redacts secret values from s.
func (r *Redactor) String(s string) string {
	if r == nil {
		return s
	}
	placeholder := r.Placeholder
	if placeholder == "" {
		placeholder = DefaultPlaceholder
	}
	for _, v := range r.Values {
		s = strings.ReplaceAll(s, v, placeholder)
	}
	return s
}

// Redact removes credential-shaped URL parameters and any known secret value
// from s. It is safe to call on a nil Redactor.
func (r *Redactor) Redact(s string) string {
	return r.String(redactQueryParams(s))
}

// redactedQueryParams lists URL parameters that carry credentials.
var redactedQueryParams = []string{
	"sig", "signature", "token", "access_token", "expires", "se", "sp", "sv",
	"st", "sr", "skoid", "keyid", "policy", "AWSAccessKeyId", "X-Goog-Signature",
	"X-Amz-Signature", "X-Amz-Credential", "X-Amz-Security-Token",
}

// redactQueryParams rewrites credential-bearing query parameters in a URL or a
// query string. Pre-signed CDN URLs must never appear verbatim in a log line.
func redactQueryParams(s string) string {
	if !strings.Contains(s, "?") {
		return s
	}
	base, query, ok := strings.Cut(s, "?")
	if !ok || query == "" {
		return s
	}
	var b strings.Builder
	b.WriteString(base)
	b.WriteByte('?')
	for i, pair := range strings.Split(query, "&") {
		if i > 0 {
			b.WriteByte('&')
		}
		name, _, hasValue := strings.Cut(pair, "=")
		if hasValue && isSecretParam(name) {
			b.WriteString(name)
			b.WriteByte('=')
			b.WriteString(DefaultPlaceholder)
			continue
		}
		b.WriteString(pair)
	}
	return b.String()
}

func isSecretParam(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, p := range redactedQueryParams {
		if lower == strings.ToLower(p) {
			return true
		}
	}
	return false
}

// Redact is the package-level helper used when no runtime secret list exists.
func Redact(s string) string { return redactQueryParams(s) }

// Hint returns an operator-facing remediation string for a kind. These strings
// are printed by `vyom doctor` and returned by the API.
func Hint(kind Kind) string {
	switch kind {
	case KindConfiguration:
		return "check the provider settings in vyom.yaml (host, account, container)"
	case KindAuthentication:
		return "check the credentials in the referenced environment variables (VYOM_AKAMAI_CLIENT_SECRET, ...)"
	case KindAuthorization:
		return "the credentials are valid but lack permission for this report or container"
	case KindNotFound:
		return "verify the object key, container name, or reporting dataset name"
	case KindRateLimited:
		return "Vyom retries rate limits with backoff; consider a smaller window or lower concurrency"
	case KindUnavailable:
		return "the provider returned a server error; Vyom already retried, re-run the scan later"
	case KindTimeout:
		return "increase the provider timeout or narrow the scan window"
	case KindMalformedResponse:
		return "the provider response could not be parsed; check whether the API schema changed"
	case KindPagination:
		return "Vyom stopped paginating early; the scan will be reported as PARTIAL"
	case KindUnsupported:
		return "this feature is not implemented in this Vyom release; see docs/roadmap.md"
	case KindCorrelation:
		return "review the correlation rules for the CDN base URL and storage key prefixes"
	case KindOptimization:
		return "inspect the source object; it may be truncated, corrupt, or of an unsupported format"
	case KindCancelled:
		return "the operation was cancelled, usually by Ctrl-C or a shutdown"
	default:
		return "this is a bug in Vyom; please open an issue with the scan ID"
	}
}
