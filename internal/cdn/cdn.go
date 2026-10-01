// Package cdn defines the CDN provider contract.
//
// Vyom is vendor-neutral: the correlation engine, the analyzer and the
// reporting layer only ever see models.TrafficRecord. Adding Cloudflare,
// Fastly or CloudFront means implementing this interface in
// internal/cdn/<name>; nothing else in the codebase changes.
//
// Implementations must be safe for concurrent use, must honour ctx
// cancellation, and must return providererr-classified errors so that the CLI
// can tell a rate limit from a bad credential.
package cdn

import (
	"context"
	"io"
	"time"

	"github.com/kaushalacts/vyom/internal/transport"
	"github.com/kaushalacts/vyom/pkg/models"
)

// Provider is a CDN integration.
//
// Interface method sets are intentionally minimal in v0.1.0: FetchTraffic is
// the only capability the scan pipeline needs. Capabilities that not every CDN
// offers (per-edge detail, origin fetch logs, cache purge) belong in separate,
// optional interfaces so that adding them does not force every provider to
// implement them.
type Provider interface {
	// Name returns the stable provider identifier, for example "akamai".
	// It appears in scan records, logs and metrics labels, so it must never
	// contain credentials or tenant identifiers.
	Name() string

	// FetchTraffic returns normalized traffic for every URL observed in the
	// half-open window [start, end).
	//
	// Implementations must:
	//   - paginate until the window is exhausted;
	//   - return records sorted or deduplicated by URL, aggregating the
	//     window for callers that do not need per-bucket granularity;
	//   - classify every failure with providererr;
	//   - stop promptly when ctx is done.
	//
	// A partially retrieved window must be returned as a providererr of kind
	// providererr.KindPagination together with the records gathered so far, so
	// the caller can persist a PARTIAL scan instead of discarding good data.
	FetchTraffic(ctx context.Context, start, end time.Time) ([]models.TrafficRecord, error)

	// Close releases long-lived resources (HTTP clients, token caches).
	// It must be safe to call more than once.
	io.Closer
}

// Granularity describes the reporting granularity a provider supports.
type Granularity string

// Reporting granularities.
const (
	// GranularityHourly: per URL per hour.
	GranularityHourly Granularity = "hourly"
	// GranularityDaily: per URL per day.
	GranularityDaily Granularity = "daily"
	// GranularityMonthly: per URL per month.
	GranularityMonthly Granularity = "monthly"
)

// Capability is an optional provider feature, discovered at runtime.
type Capability string

// Known capabilities.
const (
	// CapabilityURLDetail: traffic is reported per URL rather than per
	// aggregated path prefix. This is the capability Vyom's core value
	// proposition depends on.
	CapabilityURLDetail Capability = "url_detail"
	// CapabilityContentType: the report includes a content type dimension.
	CapabilityContentType Capability = "content_type"
	// CapabilityStatusCode: the report includes HTTP status.
	CapabilityStatusCode Capability = "status_code"
	// CapabilityGeo: the report includes a country dimension.
	CapabilityGeo Capability = "geo"
)

// Capabilities is implemented by providers that can describe themselves.
type Capabilities interface {
	// Capabilities returns the optional features the provider supports.
	Capabilities() []Capability
	// Granularity returns the finest reporting window available.
	Granularity() Granularity
}

// Describe returns the capabilities of p, or an empty set when p does not
// implement Capabilities.
func Describe(p Provider) Capabilities {
	if c, ok := p.(Capabilities); ok {
		return c
	}
	return nopCapabilities{}
}

type nopCapabilities struct{}

func (nopCapabilities) Capabilities() []Capability { return nil }
func (nopCapabilities) Granularity() Granularity   { return GranularityDaily }

// Supports reports whether p advertises the given capability.
func Supports(p Provider, c Capability) bool {
	for _, have := range Describe(p).Capabilities() {
		if have == c {
			return true
		}
	}
	return false
}

// Registry maps provider types to constructors.
//
// The registry is the seam that keeps provider-specific code out of the core:
// the scan pipeline asks for a provider by type and never learns which package
// implemented it. v0.1.0 registers only the providers that exist; see
// internal/provider/registry.go.
type Registry interface {
	// CDN returns the constructor for a CDN provider type.
	CDN(typ models.ProviderType) (Factory, bool)
	// Storage returns the constructor for a storage provider type.
	Storage(typ models.ProviderType) (Factory, bool)
}

// FactoryOptions carries runtime dependencies for a provider construction. It
// is a plain struct on purpose: adding a dependency means adding a field, not
// changing every constructor signature.
type FactoryOptions struct {
	// Config is the provider's non-secret configuration.
	Config models.ProviderConfig
	// Resolver returns the value of a secret environment variable. It is an
	// interface so that tests never touch the process environment and so that
	// secret access can be audited.
	Resolver SecretResolver
	// HTTPClient is the shared HTTP client. Sharing one client means shared
	// connection pooling and a single place to enforce timeouts.
	HTTPClient transport.Doer
	// UserAgent is sent on every outbound request.
	UserAgent string
	// Log receives provider-level diagnostics. Never pass credentials to it.
	Log transport.Logger
	// Concurrency bounds parallel work this provider performs internally.
	Concurrency int
}

// Factory constructs a provider from options.
type Factory func(ctx context.Context, opts FactoryOptions) (Provider, error)

// SecretResolver reads secret values by environment variable name.
type SecretResolver interface {
	// Lookup returns the value of name and whether it was present.
	Lookup(name string) (value string, ok bool)
}
