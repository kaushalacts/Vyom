// Package optimizer defines the image optimization contract.
//
// Vyom's optimization contract is intentionally strict, because the failure
// mode of an image optimizer is silent: a smaller file that looks worse. Every
// Optimize call therefore returns an OptimizationResult carrying SSIM and PSNR,
// and the caller is expected to reject candidates below the policy's quality
// gate rather than shipping them.
package optimizer

import (
	"context"
	"io"

	"github.com/kaushalacts/vyom/pkg/models"
)

// Policy is the resolved, validated rule set handed to an Optimizer. It is
// models.OptimizationPolicy so that the CLI, the API and the optimizer always
// agree on the same rules; internal/policy is responsible for validating it.
type Policy = models.OptimizationPolicy

// Result is an alias kept for readability at call sites.
type Result = models.OptimizationResult

// OptimizedAsset is the encoded output of Optimize.
type OptimizedAsset = models.OptimizedAsset

// Optimizer encodes media according to a policy.
//
// Implementations must:
//
//   - never mutate the input reader's contents;
//   - accept a stream, and return a stream, rather than requiring a []byte;
//   - cap memory use, rejecting inputs above the policy's MaxDownloadBytes;
//   - return an error classified as providererr.KindOptimization for
//     malformed, truncated, unsupported or corrupt input;
//   - populate SSIM and PSNR on every candidate, so the caller can enforce the
//     quality gate without trusting the encoder.
type Optimizer interface {
	// Name returns the stable optimizer identifier, for example "libvips".
	Name() string

	// Analyze reports what could be achieved without producing output: the
	// source format and dimensions, and the projected result of applying the
	// policy. Analysis never writes to storage and is safe to run on every
	// candidate asset in a scan.
	//
	// It is the call that powers "estimated optimized size" in reports. The
	// estimate is explicitly an estimate: it is derived from the policy and
	// the source properties, not from encoding the image.
	Analyze(ctx context.Context, input io.Reader, policy Policy) (*models.OptimizationResult, error)

	// Optimize encodes input according to policy and returns the candidate.
	//
	// The returned result has Rejected=true and a Reason when the candidate
	// fails the policy's SSIM/PSNR gate. A rejection is not an error: it is a
	// decision, and the caller is expected to record it and move on. Only
	// genuinely impossible work (corrupt input, unsupported format, encoder
	// failure) returns an error.
	Optimize(ctx context.Context, input io.Reader, policy Policy) (*models.OptimizedAsset, error)

	io.Closer
}

// Capabilities describes what an Optimizer can encode.
type Capabilities interface {
	// Supports reports whether the format can be used as a source or target.
	Supports(format models.MediaFormat) bool
	// Encodes reports whether the format can be produced as output.
	Encodes(format models.MediaFormat) bool
	// Metrics reports which quality metrics this optimizer computes.
	Metrics() []Metric
}

// Metric is a quality metric an Optimizer can compute.
type Metric string

// Known quality metrics.
const (
	MetricSSIM Metric = "ssim"
	MetricPSNR Metric = "psnr"
)

// Availability reports whether o advertises m.
func Availability(o Optimizer, m Metric) bool {
	c, ok := o.(Capabilities)
	if !ok {
		return false
	}
	for _, have := range c.Metrics() {
		if have == m {
			return true
		}
	}
	return false
}

// Anomaly is a structural difference between the original and a candidate that
// is worth a human's attention even when SSIM and PSNR pass.
//
// SSIM and PSNR measure pixel fidelity on average. They miss systematic
// differences: a stripped EXIF orientation, a removed colour profile, a
// dropped alpha channel, a stripped animation. Vyom records these so that a
// policy can require them rather than discovering them in production.
type Anomaly struct {
	// Kind is a short identifier, for example "alpha_dropped".
	Kind string `json:"kind"`
	// Detail explains the difference in human terms.
	Detail string `json:"detail"`
	// Severity is "info", "warning" or "critical".
	Severity string `json:"severity"`
}

// Anomaly severities.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Inspectable is implemented by optimizers that can compare a source and a
// candidate and report anomalies beyond SSIM and PSNR.
type Inspectable interface {
	// Compare inspects a candidate against the source and returns any
	// anomalies. It returns nil when the candidate is structurally faithful.
	Compare(ctx context.Context, original, candidate io.Reader, policy Policy) ([]Anomaly, error)
}

// Registry maps optimizer names to constructors.
type Registry interface {
	// Get returns the constructor for an optimizer name.
	Get(name string) (Factory, bool)
}

// Factory constructs an Optimizer.
type Factory func(ctx context.Context, opts Options) (Optimizer, error)

// Options configures an Optimizer construction.
type Options struct {
	// Concurrency bounds parallel encodes. libvips is memory-hungry: each
	// concurrent 8K decode can hold hundreds of megabytes, so Vyom's default
	// is deliberately small and configurable.
	Concurrency int
	// MaxPixels rejects images whose pixel count exceeds this, guarding
	// against decompression-bomb style inputs.
	MaxPixels int64
	// TempDir is where intermediate files are written, when the backend needs
	// them. Empty means the OS temp directory.
	TempDir string
}

// DefaultMaxPixels is 100 megapixels, roughly a 12000x8300 image. Beyond
// that, a single decode is a practical denial-of-service against a small
// worker.
const DefaultMaxPixels int64 = 100_000_000

// DefaultOptions returns Vyom's optimizer defaults.
func DefaultOptions() Options {
	return Options{Concurrency: 2, MaxPixels: DefaultMaxPixels}
}
