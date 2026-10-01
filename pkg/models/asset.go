package models

import (
	"fmt"
	"time"
)

// Asset is a media object that Vyom knows about, after CDN traffic has been
// correlated with a storage object.
//
// A single asset may be observed under several CDN URLs (query-string
// variants, cache-busting, multiple hostnames). Traffic for those URLs is
// aggregated onto one Asset, identified by StorageKey.
type Asset struct {
	// ID is Vyom's stable identifier for the asset (hex, 32 chars).
	ID string `json:"id"`
	// URL is the canonical CDN URL of the asset.
	URL string `json:"url"`
	// StorageKey is the object key inside the storage provider's container.
	StorageKey string `json:"storage_key"`
	// StorageProvider is the provider that owns StorageKey (for example
	// "azure_blob").
	StorageProvider string `json:"storage_provider,omitempty"`
	// CDNProvider is the provider that served the traffic (for example "akamai").
	CDNProvider string `json:"cdn_provider,omitempty"`
	// ContentType is the stored object's content type.
	ContentType string `json:"content_type,omitempty"`
	// Format is the normalized media format, for example "jpeg".
	Format string `json:"format,omitempty"`
	// SizeBytes is the size of the stored object.
	SizeBytes int64 `json:"size_bytes"`
	// Width and Height are pixel dimensions of the media, 0 when unknown.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Requests is the number of CDN requests observed in the scan period.
	Requests int64 `json:"requests"`
	// BytesDelivered is the number of bytes served from the CDN in the scan
	// period. This is the number the whole product ranks on.
	BytesDelivered int64 `json:"bytes_delivered"`
	// FirstSeen and LastSeen bound the traffic window of the scan.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	// LastModified is the storage object's last modification time.
	LastModified time.Time `json:"last_modified,omitempty"`
	// ETag is the storage object's entity tag, used for change detection.
	ETag string `json:"etag,omitempty"`
	// ContentHash is the SHA-256 of the object bytes (hex). Empty until the
	// object has been downloaded for analysis or optimization.
	ContentHash string `json:"content_hash,omitempty"`
	// ScanID is the scan that last updated this asset.
	ScanID string `json:"scan_id,omitempty"`
}

// Pixels returns the pixel count of the asset, or 0 when dimensions are
// unknown.
func (a Asset) Pixels() int64 {
	if a.Width <= 0 || a.Height <= 0 {
		return 0
	}
	return int64(a.Width) * int64(a.Height)
}

// IsMedia reports whether the asset is a media object Vyom can analyze.
func (a Asset) IsMedia() bool { return MediaFormat(a.Format).IsKnown() }

// AverageSizeBytes returns the average transferred size per request. It is a
// useful sanity signal: an average far above SizeBytes usually means several
// variants of the same asset are being served.
func (a Asset) AverageSizeBytes() int64 {
	if a.Requests <= 0 {
		return 0
	}
	return a.BytesDelivered / a.Requests
}

// TrafficRecord is normalized per-URL CDN traffic for a time window.
//
// Providers report traffic at different granularities (hourly, daily, per
// edge location). Vyom normalizes everything to this shape.
type TrafficRecord struct {
	// URL is the CDN URL as observed by the provider.
	URL string `json:"url"`
	// Requests is the number of requests for URL in the window.
	Requests int64 `json:"requests"`
	// BytesDelivered is the number of bytes served for URL in the window.
	BytesDelivered int64 `json:"bytes_delivered"`
	// StartTime and EndTime bound the window (UTC, inclusive/exclusive).
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	// ContentType is the content type reported by the CDN, when available.
	ContentType string `json:"content_type,omitempty"`
	// Format is the normalized media format derived by the provider adapter.
	Format string `json:"format,omitempty"`
	// StatusCode is the aggregated HTTP status, when the provider reports one.
	StatusCode int `json:"status_code,omitempty"`
	// Country is the aggregated client country, when the provider reports one.
	Country string `json:"country,omitempty"`
	// Provider is the CDN that produced this record.
	Provider string `json:"provider,omitempty"`
}

// Validate performs basic sanity checks used before persisting traffic.
func (t TrafficRecord) Validate() error {
	if t.URL == "" {
		return fmt.Errorf("traffic record: url must not be empty")
	}
	if t.Requests < 0 {
		return fmt.Errorf("traffic record %s: requests must not be negative", t.URL)
	}
	if t.BytesDelivered < 0 {
		return fmt.Errorf("traffic record %s: bytes_delivered must not be negative", t.URL)
	}
	if !t.EndTime.IsZero() && !t.StartTime.IsZero() && t.EndTime.Before(t.StartTime) {
		return fmt.Errorf("traffic record %s: end_time %s is before start_time %s",
			t.URL, t.EndTime.UTC().Format(time.RFC3339), t.StartTime.UTC().Format(time.RFC3339))
	}
	return nil
}

// ObjectMetadata describes a single object in a storage container.
type ObjectMetadata struct {
	// ID is Vyom's stable identifier for the object (hex, 32 chars).
	ID string `json:"id"`
	// Key is the object key relative to the container root.
	Key string `json:"key"`
	// URL is the publicly reachable URL of the object, when derivable.
	URL string `json:"url,omitempty"`
	// SizeBytes is the object size.
	SizeBytes int64 `json:"size_bytes"`
	// ContentType is the object's content type.
	ContentType string `json:"content_type,omitempty"`
	// LastModified is the object's last modification time.
	LastModified time.Time `json:"last_modified"`
	// ETag is the object's entity tag.
	ETag string `json:"etag,omitempty"`
	// BlobType and StorageClass are Azure-specific but normalized to plain
	// strings so that other providers can populate them with their own terms.
	BlobType     string `json:"blob_type,omitempty"`
	StorageClass string `json:"storage_class,omitempty"`
	// Provider is the storage provider that owns the object.
	Provider string `json:"provider,omitempty"`
	// Container is the container or bucket holding the object.
	Container string `json:"container,omitempty"`
	// ScanID is the scan that last updated this object.
	ScanID string `json:"scan_id,omitempty"`
}

// Validate performs basic sanity checks used before persisting object metadata.
func (o ObjectMetadata) Validate() error {
	if o.Key == "" {
		return fmt.Errorf("object metadata: key must not be empty")
	}
	if o.SizeBytes < 0 {
		return fmt.Errorf("object metadata %s: size_bytes must not be negative", o.Key)
	}
	return nil
}

// OptimizationResult describes one analyzed or optimized image.
//
// Vyom never reports a reduction without the quality metrics that justify it.
// A 70% smaller image with SSIM 0.6 is not a successful optimization, it is a
// different image.
type OptimizationResult struct {
	OriginalSize     int64   `json:"original_size"`
	OptimizedSize    int64   `json:"optimized_size"`
	ReductionPercent float64 `json:"reduction_percent"`
	OriginalFormat   string  `json:"original_format"`
	OptimizedFormat  string  `json:"optimized_format"`
	// SSIM is the structural similarity index in [0,1]. 0 when not computed.
	SSIM float64 `json:"ssim"`
	// PSNR is the peak signal-to-noise ratio in decibels. 0 when not computed.
	PSNR float64 `json:"psnr"`
	// Width and Height are the optimized dimensions.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// EncodedWidth/EncodedHeight are the source dimensions, before resize.
	EncodedWidth  int `json:"encoded_width,omitempty"`
	EncodedHeight int `json:"encoded_height,omitempty"`
	// Quality is the encoder quality setting used (1-100), 0 if not set.
	Quality int `json:"quality,omitempty"`
	// OriginalHash and OptimizedHash are SHA-256 hex digests of the bytes.
	OriginalHash  string `json:"original_hash,omitempty"`
	OptimizedHash string `json:"optimized_hash,omitempty"`
	// Rejected is set when a candidate failed the policy's quality gate.
	Rejected bool `json:"rejected,omitempty"`
	// Reason explains a rejection, for example "ssim 0.91 below minimum 0.95".
	Reason string `json:"reason,omitempty"`
	// DurationMS is the wall-clock time spent producing the candidate.
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// Improved reports whether the candidate is meaningfully smaller than the
// original. Vyom requires at least 5% to consider a rewrite worthwhile, which
// avoids churning assets for negligible gains.
func (r *OptimizationResult) Improved() bool {
	if r == nil || r.Rejected {
		return false
	}
	return r.ReductionPercent >= MinMeaningfulReductionPercent
}

// OptimizedAsset is the encoded result of an optimization, ready to be
// persisted under a new key. Vyom never deletes or overwrites the original.
type OptimizedAsset struct {
	// StorageKey is the destination key for the optimized object.
	StorageKey string `json:"storage_key"`
	// ContentType is the content type of the encoded bytes.
	ContentType string `json:"content_type"`
	// Data holds the encoded bytes. It is never serialized to the API.
	Data []byte `json:"-"`
	// Result carries the size and quality metrics of the encode.
	Result OptimizationResult `json:"result"`
}

// MinMeaningfulReductionPercent is the smallest size reduction Vyom considers
// worth applying. Below this, the visual risk of re-encoding outweighs the
// bandwidth saved.
const MinMeaningfulReductionPercent = 5.0
