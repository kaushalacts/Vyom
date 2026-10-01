package models

import "time"

// OptimizationPolicy is a versioned, named set of rules that decides which
// assets are optimization opportunities and how they should be re-encoded.
//
// Policies are data, not code: they are stored in the database, referenced by
// scans and optimization jobs, and shipped as YAML files under configs/policies.
type OptimizationPolicy struct {
	// Name identifies the policy (for example "default", "ecommerce").
	Name string `json:"name"`
	// Version is the semantic version of the policy document. Changing a
	// policy requires bumping the version so historical results stay
	// reproducible.
	Version string `json:"version"`
	// Description is a human-readable summary shown by `vyom policy list`.
	Description string `json:"description,omitempty"`
	// Images holds the rules for still images (JPEG, PNG, WebP).
	Images ImagePolicy `json:"images"`
	// Video holds placeholder rules for future video support. Vyom v0.1.0
	// records video traffic and reports recommendations only; it never
	// transcodes.
	Video VideoPolicy `json:"video,omitempty"`
	// Pricing is optional per-policy pricing, used when global pricing is unset.
	Pricing PricingConfig `json:"pricing,omitempty"`
}

// ImagePolicy holds the still-image rules.
//
// Every threshold is optional: an unset threshold means "no limit" rather than
// zero, so that a minimal policy is valid.
type ImagePolicy struct {
	// WarningSizeBytes marks assets as oversized for reporting only.
	WarningSizeBytes int64 `json:"warning_size_bytes,omitempty"`
	// CriticalSizeBytes marks assets as high impact.
	CriticalSizeBytes int64 `json:"critical_size_bytes,omitempty"`
	// MaxBytes rejects candidates larger than this, even when smaller than
	// the original.
	MaxBytes int64 `json:"max_bytes,omitempty"`
	// MaxWidth and MaxHeight cap the delivered dimensions.
	MaxWidth  int `json:"max_width,omitempty"`
	MaxHeight int `json:"max_height,omitempty"`
	// TargetFormat is the format candidates are encoded to, for example "webp".
	TargetFormat string `json:"target_format,omitempty"`
	// Quality is the encoder quality setting (1-100).
	Quality int `json:"quality,omitempty"`
	// MinimumSSIM and MinimumPSNR are the quality gates. A candidate that
	// fails either is rejected, not shipped.
	MinimumSSIM float64 `json:"minimum_ssim,omitempty"`
	MinimumPSNR float64 `json:"minimum_psnr,omitempty"`
	// AllowedFormats restricts which source formats the policy applies to.
	// Empty means all formats Vyom supports.
	AllowedFormats []string `json:"allowed_formats,omitempty"`
	// MinRequests prevents optimizing assets with negligible traffic. This is
	// what keeps Vyom from spending hours re-encoding assets nobody requests.
	MinRequests int64 `json:"min_requests,omitempty"`
	// MinDeliveredBytes is an alternative traffic gate.
	MinDeliveredBytes int64 `json:"min_delivered_bytes,omitempty"`
	// MinReductionPercent is the smallest reduction worth applying.
	MinReductionPercent float64 `json:"min_reduction_percent,omitempty"`
	// MaxDownloadBytes rejects objects larger than this before download.
	MaxDownloadBytes int64 `json:"max_download_bytes,omitempty"`
	// PreserveMetadata keeps EXIF and ICC profiles when re-encoding.
	PreserveMetadata bool `json:"preserve_metadata,omitempty"`
}

// VideoPolicy records the future video behaviour. In v0.1.0 Vyom only uses it
// to produce recommendations; no transcoding is performed.
type VideoPolicy struct {
	// MaxBitrateKbps and TargetBitrateKbps describe the recommendation model.
	MaxBitrateKbps    int64 `json:"max_bitrate_kbps,omitempty"`
	TargetBitrateKbps int64 `json:"target_bitrate_kbps,omitempty"`
	// TargetFormat is a future output container, for example "h265".
	TargetFormat string `json:"target_format,omitempty"`
	// ResolutionLadder maps input resolutions to recommended ones.
	ResolutionLadder map[string]string `json:"resolution_ladder,omitempty"`
}

// PricingConfig is optional pricing input. When UnitPricePerGB is zero, Vyom
// refuses to print monetary figures rather than inventing them.
type PricingConfig struct {
	// Currency is an ISO-4217 code, for example "USD".
	Currency string `json:"currency,omitempty"`
	// UnitPricePerGB is the price of one decimal GB (10^9 bytes).
	UnitPricePerGB float64 `json:"bandwidth_per_gb,omitempty"`
	// StoragePerGBMonth optionally prices object storage for the
	// optimization opportunities (storing optimized copies).
	StoragePerGBMonth float64 `json:"storage_per_gb_month,omitempty"`
	// PerRequest optionally prices requests.
	PerRequest float64 `json:"per_request,omitempty"`
}

// Configured reports whether any price was supplied.
func (p PricingConfig) Configured() bool {
	return p.UnitPricePerGB > 0 || p.StoragePerGBMonth > 0 || p.PerRequest > 0
}

// OptimizationJobStatus is the state of an optimization run.
type OptimizationJobStatus string

// Optimization job statuses.
const (
	OptStatusPending    OptimizationJobStatus = "pending"
	OptStatusRunning    OptimizationJobStatus = "running"
	OptStatusCompleted  OptimizationJobStatus = "completed"
	OptStatusPartial    OptimizationJobStatus = "partial"
	OptStatusFailed     OptimizationJobStatus = "failed"
	OptStatusCancelled  OptimizationJobStatus = "cancelled"
	OptStatusRolledBack OptimizationJobStatus = "rolled_back"
)

// ManifestOperation is what Vyom did (or will do) to a storage object.
//
// Vyom never overwrites and never deletes an original. The only supported
// operation is generating a new, optimized copy alongside the original.
type ManifestOperation string

// Manifest operations.
const (
	// OpGenerateCopy writes <name>.<ext> next to the original and leaves the
	// original untouched.
	OpGenerateCopy ManifestOperation = "generate-copy"
	// OpDeleteCopy removes a previously generated copy during rollback.
	OpDeleteCopy ManifestOperation = "delete-copy"
	// OpSkipped records an asset that was considered and left unchanged.
	OpSkipped ManifestOperation = "skipped"
	// OpFailed records an asset whose optimization failed.
	OpFailed ManifestOperation = "failed"
)

// ManifestStatus is the per-asset result inside a manifest.
type ManifestStatus string

// Manifest statuses.
const (
	ManifestStatusSuccess    ManifestStatus = "success"
	ManifestStatusSkipped    ManifestStatus = "skipped"
	ManifestStatusFailed     ManifestStatus = "failed"
	ManifestStatusRolledBack ManifestStatus = "rolled_back"
)

// ManifestEntry is one line of an optimization manifest.
//
// The manifest is the audit record that makes --apply safe: every modified
// object has a matching entry with the original key, the new key, the hashes
// and the quality metrics, which is exactly what a rollback needs.
type ManifestEntry struct {
	// ID is Vyom's stable identifier for the manifest entry (hex, 32 chars).
	ID string `json:"id"`
	// JobID groups entries belonging to one optimization run.
	JobID string `json:"job_id"`
	// Asset references the asset that was optimized.
	Asset string `json:"asset"`
	// OriginalSize and OptimizedSize are in bytes.
	OriginalSize  int64 `json:"original_size"`
	OptimizedSize int64 `json:"optimized_size"`
	// OriginalFormat and OptimizedFormat are normalized format names.
	OriginalFormat  string `json:"original_format"`
	OptimizedFormat string `json:"optimized_format"`
	// SSIM and PSNR justify the encode.
	SSIM float64 `json:"ssim"`
	PSNR float64 `json:"psnr"`
	// OriginalKey and OptimizedKey are the storage keys involved.
	OriginalKey  string `json:"original_key,omitempty"`
	OptimizedKey string `json:"optimized_key,omitempty"`
	// OriginalHash and OptimizedHash are SHA-256 hex digests.
	OriginalHash  string `json:"original_hash,omitempty"`
	OptimizedHash string `json:"optimized_hash,omitempty"`
	// Requests and BytesDelivered are the traffic this entry refers to.
	Requests       int64 `json:"requests,omitempty"`
	BytesDelivered int64 `json:"bytes_delivered,omitempty"`
	// EstimatedSavingsBytes is the projected bandwidth reduction.
	EstimatedSavingsBytes int64 `json:"estimated_savings_bytes,omitempty"`
	// Timestamp is when the operation ran (UTC).
	Timestamp time.Time `json:"timestamp"`
	// Operation and Status describe what happened.
	Operation ManifestOperation `json:"operation"`
	Status    ManifestStatus    `json:"status"`
	// DryRun marks entries produced by a simulation, where nothing was written.
	DryRun bool `json:"dry_run"`
	// Reason explains skipped or failed entries.
	Reason string `json:"reason,omitempty"`
}

// ReductionPercent returns the size reduction for the entry.
func (e ManifestEntry) ReductionPercent() float64 {
	if e.OriginalSize <= 0 {
		return 0
	}
	return float64(e.OriginalSize-e.OptimizedSize) / float64(e.OriginalSize) * 100
}

// OptimizationJob is one optimization run. In v0.1.0 every job is a
// simulation: DryRun is always true and no object is modified.
type OptimizationJob struct {
	ID string `json:"id"`
	// DryRun is true for simulations. It defaults to true and can only become
	// false through an explicit --apply with confirmation.
	DryRun     bool                  `json:"dry_run"`
	Status     OptimizationJobStatus `json:"status"`
	Policy     string                `json:"policy,omitempty"`
	ScanID     string                `json:"scan_id,omitempty"`
	Counts     OptimizationCounts    `json:"counts"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
	Error      string                `json:"error,omitempty"`
}

// OptimizationCounts summarizes an optimization run.
type OptimizationCounts struct {
	// Considered is how many assets the policy selected.
	Considered int64 `json:"considered"`
	// Skipped counts assets left unchanged (too small, too few requests,
	// quality gate failed, already optimized).
	Skipped int64 `json:"skipped"`
	// Candidates counts assets that produced a valid, smaller candidate.
	Candidates int64 `json:"candidates"`
	// Rejected counts candidates that failed the quality gate.
	Rejected int64 `json:"rejected"`
	// Failed counts assets whose processing errored.
	Failed int64 `json:"failed"`
	// Written counts objects actually written (always 0 in dry-run).
	Written int64 `json:"written"`
	// BytesBefore, BytesAfter and BytesSaved describe the projected change.
	BytesBefore int64 `json:"bytes_before"`
	BytesAfter  int64 `json:"bytes_after"`
	BytesSaved  int64 `json:"bytes_saved"`
	// DeliveredBytesBefore and DeliveredAfter are traffic projections over
	// the scan window, not object sizes.
	DeliveredBytesBefore int64 `json:"delivered_bytes_before"`
	DeliveredBytesAfter  int64 `json:"delivered_bytes_after"`
}
