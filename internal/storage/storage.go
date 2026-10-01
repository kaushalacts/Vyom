// Package storage defines the object storage provider contract.
//
// The contract is read-only in v0.1.0. Vyom inventories origin objects,
// downloads the ones a policy selects for analysis and, in a later milestone,
// writes optimized copies. It never deletes or overwrites an original, so this
// package deliberately has no Delete or Put operation today.
package storage

import (
	"context"
	"io"

	"github.com/kaushalacts/vyom/internal/transport"
	"github.com/kaushalacts/vyom/pkg/models"
)

// Provider is an object storage integration.
type Provider interface {
	// Name returns the stable provider identifier, for example "azure_blob".
	Name() string

	// ListObjects returns metadata for every object in the configured
	// container or bucket.
	//
	// Implementations must:
	//   - paginate server-side, never materialize a full listing in memory
	//     when the container can hold millions of blobs;
	//   - return models.ObjectMetadata with a normalized Provider and
	//     Container, and SizeBytes populated;
	//   - honour ctx cancellation between pages;
	//   - classify failures with providererr.
	ListObjects(ctx context.Context) ([]models.ObjectMetadata, error)

	// GetMetadata returns metadata for a single key. A missing key must
	// return an error classified as providererr.KindNotFound so callers can
	// skip the object rather than abort a scan.
	GetMetadata(ctx context.Context, key string) (*models.ObjectMetadata, error)

	// Download returns a reader over the object's bytes.
	//
	// Implementations MUST enforce MaxDownloadSize. Returning an unbounded
	// stream for an arbitrary key is a denial-of-service vector, because the
	// caller then buffers it to compute an image's dimensions.
	Download(ctx context.Context, key string, opts DownloadOptions) (io.ReadCloser, error)

	io.Closer
}

// DownloadOptions bounds a download.
type DownloadOptions struct {
	// MaxDownloadSize rejects objects larger than this many bytes. Zero means
	// the provider's configured default applies. Providers must never stream
	// an unbounded object.
	MaxDownloadSize int64
	// Range requests a byte range. Both zero means the whole object.
	Offset int64
	Length int64
}

// WholeObject reports whether the options request the entire object.
func (o DownloadOptions) WholeObject() bool { return o.Offset == 0 && o.Length == 0 }

// Writer is implemented by storage providers that can write new objects.
//
// It is a separate interface so that read-only credentials stay valid: a
// deployment that only runs scans can be granted read-only access, and Vyom
// will not even be able to construct a writer.
type Writer interface {
	// Put writes data at key, without overwriting an existing object unless
	// overwrite is true. Vyom always passes overwrite=false and never deletes
	// originals.
	Put(ctx context.Context, key string, data io.Reader, size int64, contentType string, overwrite bool) (*models.ObjectMetadata, error)
}

// Deleter is implemented by providers that support deletion.
//
// Vyom v0.1.0 never calls it. It exists so that rollback in a later milestone
// can delete a previously generated copy, and so that the absence of deletion
// support is a documented capability rather than a silent surprise.
type Deleter interface {
	Delete(ctx context.Context, key string) error
}

// Capabilities describes optional storage features.
type Capabilities interface {
	// SupportsServerSideCopy reports whether an object can be copied without
	// downloading it. Relevant to "keep the original, publish a variant".
	SupportsServerSideCopy() bool
	// MaxDownloadSize returns the hard limit the provider enforces.
	MaxDownloadSize() int64
}

// DefaultMaxDownloadBytes is Vyom's default ceiling for a single object
// analysis. 100 MB is far above any sane product image and below the point
// where a decode can threaten an operator's memory.
const DefaultMaxDownloadBytes int64 = 100 << 20 // 100 MiB

// DefaultDownloadOptions returns options carrying the default ceiling.
func DefaultDownloadOptions() DownloadOptions {
	return DownloadOptions{MaxDownloadSize: DefaultMaxDownloadBytes}
}

// FactoryOptions carries runtime dependencies for a provider construction.
type FactoryOptions struct {
	// Config is the provider's non-secret configuration.
	Config models.ProviderConfig
	// Resolver reads secret values by environment variable name.
	Resolver SecretResolver
	// HTTPClient is the shared HTTP client.
	HTTPClient transport.Doer
	// UserAgent is sent on every outbound request.
	UserAgent string
	// Log receives provider-level diagnostics. Never pass credentials to it.
	Log transport.Logger
	// Concurrency bounds parallel work this provider performs internally.
	Concurrency int
	// MaxDownloadSize overrides DefaultMaxDownloadBytes.
	MaxDownloadSize int64
}

// Factory constructs a storage provider from options.
type Factory func(ctx context.Context, opts FactoryOptions) (Provider, error)

// SecretResolver reads secret values by environment variable name.
type SecretResolver interface {
	// Lookup returns the value of name and whether it was present.
	Lookup(name string) (value string, ok bool)
}
