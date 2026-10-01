// Package policy owns optimization policy definitions, validation, evaluation
// and impact scoring.
//
// Two design decisions drive this package.
//
// First, Vyom never optimizes on a size threshold alone. A 900 KB image served
// ten times a month is not a problem; the same image served five million times
// is. Impact is therefore computed relative to the whole scanned corpus, using
// the formulas documented on ComputeImpact and PriorityScore.
//
// Second, policies are data. They are stored in the database, referenced by
// scan and optimization jobs, versioned, and shipped as YAML. Code changes are
// never required to change a threshold.
package policy

import (
	"fmt"
	"math"
	"strings"

	"github.com/kaushalacts/vyom/pkg/models"
)

// DefaultName is the policy used when the user does not select one.
const DefaultName = "default"

// DefaultVersion is the semantic version of Vyom's built-in default policy.
const DefaultVersion = "1.0.0"

// Set is a named, versioned collection of policies.
type Set struct {
	// Name is the set's identifier, usually the file name without extension.
	Name string `yaml:"name" json:"name"`
	// Policies holds one or more named policies.
	Policies []models.OptimizationPolicy `yaml:"policies" json:"policies"`
	// Pricing is the set-level default pricing, overridable per policy.
	Pricing models.PricingConfig `yaml:"pricing" json:"pricing"`
}

// DefaultMinRequests is Vyom's default traffic floor.
//
// Without a traffic floor, every oversized object becomes an "opportunity"
// regardless of how often it is served, which is the exact mistake Vyom exists
// to avoid: a 900 KB hero image requested ten times a month is a non-event, and
// re-encoding it costs an operator's time to save a few megabytes. 1,000
// requests per scan window is low enough to catch any real contributor and high
// enough to exclude incidental assets.
//
// Tune it from your own scan rather than guessing; see docs/policies.md.
const DefaultMinRequests int64 = 1000

// Default returns Vyom's built-in default policy.
//
// The thresholds are deliberately conservative. They exist so that a fresh
// install produces a sensible report, not to express an opinion about anyone's
// assets; docs/policies.md explains how to choose real values from a scan.
func Default() models.OptimizationPolicy {
	return models.OptimizationPolicy{
		Name:        DefaultName,
		Version:     DefaultVersion,
		Description: "Vyom built-in defaults: report-only, 200 KB warning, 500 KB critical, WebP at quality 80, minimum SSIM 0.95, 1000-request traffic floor.",
		Images: models.ImagePolicy{
			WarningSizeBytes:    200 * 1024,
			CriticalSizeBytes:   500 * 1024,
			MaxWidth:            1920,
			MaxHeight:           1920,
			TargetFormat:        string(models.FormatWebP),
			Quality:             80,
			MinimumSSIM:         0.95,
			MinimumPSNR:         30,
			MinRequests:         DefaultMinRequests,
			MinReductionPercent: models.MinMeaningfulReductionPercent,
			MaxDownloadBytes:    100 << 20,
		},
		Video: models.VideoPolicy{
			MaxBitrateKbps:    20000,
			TargetBitrateKbps: 6000,
			TargetFormat:      "h265",
			ResolutionLadder: map[string]string{
				"2160p": "1080p",
				"1440p": "1080p",
				"1080p": "1080p",
			},
		},
	}
}

// DefaultSet returns a policy set containing only the default policy.
func DefaultSet() Set {
	return Set{Name: DefaultName, Policies: []models.OptimizationPolicy{Default()}}
}

// Lookup returns the named policy from the set.
func (s Set) Lookup(name string) (models.OptimizationPolicy, bool) {
	if name == "" || name == DefaultName {
		if len(s.Policies) == 0 {
			return Default(), true
		}
		return s.Policies[0], true
	}
	for _, p := range s.Policies {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return models.OptimizationPolicy{}, false
}

// Names lists the policy names in the set.
func (s Set) Names() []string {
	out := make([]string, 0, len(s.Policies))
	for _, p := range s.Policies {
		out = append(out, p.Name)
	}
	return out
}

// Validate checks a policy for internal consistency.
//
// It returns every problem it finds, with the exact YAML path, because a policy
// that silently degrades quality is worse than one that refuses to load.
func Validate(p models.OptimizationPolicy) error {
	var errs models.ValidationErrors
	prefix := "policy"

	if strings.TrimSpace(p.Name) == "" {
		errs.Addf(prefix+".name", "is required")
	}
	if strings.TrimSpace(p.Version) == "" {
		errs.Addf(prefix+".version", "is required so results stay reproducible")
	}
	if err := validateImagePolicy(p.Images, prefix+".images"); err != nil {
		var agg *models.ValidationErrors
		if asAggregate(err, &agg) {
			errs.Errors = append(errs.Errors, agg.Errors...)
		} else {
			errs.Add(err)
		}
	}
	if p.Pricing.UnitPricePerGB < 0 {
		errs.Addf(prefix+".pricing.bandwidth_per_gb", "must not be negative")
	}
	if p.Pricing.StoragePerGBMonth < 0 || p.Pricing.PerRequest < 0 {
		errs.Addf(prefix+".pricing", "prices must not be negative")
	}
	return errs.Err()
}

func validateImagePolicy(img models.ImagePolicy, prefix string) error {
	var errs models.ValidationErrors

	if img.WarningSizeBytes < 0 {
		errs.Addf(prefix+".warning_size", "must not be negative")
	}
	if img.CriticalSizeBytes < 0 {
		errs.Addf(prefix+".critical_size", "must not be negative")
	}
	if img.WarningSizeBytes > 0 && img.CriticalSizeBytes > 0 && img.CriticalSizeBytes < img.WarningSizeBytes {
		errs.Addf(prefix+".critical_size", "must be greater than or equal to warning_size (%d)", img.WarningSizeBytes)
	}
	if img.MaxBytes < 0 {
		errs.Addf(prefix+".max_bytes", "must not be negative")
	}
	if img.MaxWidth < 0 || img.MaxHeight < 0 {
		errs.Addf(prefix+".max_dimensions", "must not be negative")
	}
	if img.Quality != 0 && (img.Quality < 1 || img.Quality > 100) {
		errs.Addf(prefix+".quality", "must be between 1 and 100, got %d", img.Quality)
	}
	if img.MinimumSSIM != 0 && (img.MinimumSSIM < 0 || img.MinimumSSIM > 1) {
		errs.Addf(prefix+".minimum_ssim", "must be between 0 and 1, got %v", img.MinimumSSIM)
	}
	if img.MinimumSSIM != 0 && img.MinimumSSIM < 0.8 {
		errs.Addf(prefix+".minimum_ssim", "below 0.8 means visible degradation for most assets; prefer 0.95 or higher")
	}
	if img.MinimumPSNR < 0 {
		errs.Addf(prefix+".minimum_psnr", "must not be negative")
	}
	if img.MinReductionPercent < 0 || img.MinReductionPercent > 100 {
		errs.Addf(prefix+".min_reduction_percent", "must be between 0 and 100")
	}
	if img.MinRequests < 0 {
		errs.Addf(prefix+".min_requests", "must not be negative")
	}
	if img.MinDeliveredBytes < 0 {
		errs.Addf(prefix+".min_delivered_bytes", "must not be negative")
	}
	if img.MaxDownloadBytes < 0 {
		errs.Addf(prefix+".max_download_bytes", "must not be negative")
	}

	if tf := strings.TrimSpace(img.TargetFormat); tf != "" {
		f := models.MediaFromContentType(tf)
		if !f.IsKnown() {
			errs.Addf(prefix+".target_format",
				"%q is not a format Vyom can encode in v0.1.0 (supported: jpeg, png, webp)", tf)
		}
	}
	for _, af := range img.AllowedFormats {
		f := models.MediaFromContentType(af)
		if !f.IsKnown() {
			errs.Addf(prefix+".allowed_formats", "%q is not a supported source format (supported: jpeg, png, webp)", af)
		}
	}
	return errs.Err()
}

// asAggregate is a small errors.As helper for the ValidationErrors aggregate.
func asAggregate(err error, target **models.ValidationErrors) bool {
	for err != nil {
		if agg, ok := err.(*models.ValidationErrors); ok {
			*target = agg
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// Decision is the outcome of evaluating a policy against one asset.
type Decision struct {
	// Action is what Vyom recommends doing.
	Action Action
	// Class is the severity bucket for reporting.
	Class models.ImpactClass
	// Reasons are human-readable explanations, shown in reports and the UI.
	Reasons []string
	// TargetFormat is the format an optimization would produce.
	TargetFormat string
	// EstimatedOptimizedSize is the projected encoded size, 0 when unknown.
	EstimatedOptimizedSize int64
	// EstimatedReductionPercent is the projected reduction, 0 when unknown.
	EstimatedReductionPercent float64
}

// Action is Vyom's recommendation for an asset.
type Action string

// Actions.
const (
	// ActionIgnore: the asset is not worth attention.
	ActionIgnore Action = "ignore"
	// ActionReview: the asset is oversized but below the traffic floor.
	ActionReview Action = "review"
	// ActionOptimize: an optimization opportunity.
	ActionOptimize Action = "optimize"
	// ActionAlreadyOptimized: the asset already meets the policy.
	ActionAlreadyOptimized Action = "already_optimized"
)

// Opportunity reports whether the decision calls for optimization.
func (d Decision) Opportunity() bool { return d.Action == ActionOptimize }

// Evaluate applies the policy to an asset and returns a decision with reasons.
//
// Evaluation is pure and side-effect free: it never downloads or encodes, so it
// can run across millions of rows from stored metadata alone. Size-based
// verdicts use thresholds; whether the asset matters is decided later by
// ComputeImpact, which needs the whole corpus.
func Evaluate(p models.OptimizationPolicy, a models.Asset) Decision {
	d := Decision{Action: ActionIgnore, TargetFormat: p.Images.TargetFormat}
	img := p.Images

	format := models.MediaFormat(a.Format)
	if !format.IsKnown() {
		d.Reasons = append(d.Reasons, fmt.Sprintf("format %q is not analyzable by Vyom v0.1.0", format))
		return d
	}
	if !formatAllowed(img.AllowedFormats, format) {
		d.Reasons = append(d.Reasons, fmt.Sprintf("format %q is not in the policy's allowed_formats", format))
		return d
	}
	if !mediaPolicyAllowed(p, format) {
		d.Reasons = append(d.Reasons, fmt.Sprintf("no policy is defined for %q in this policy set", format))
		return d
	}

	// Dimensions.
	resizeNeeded := false
	if img.MaxWidth > 0 && a.Width > img.MaxWidth {
		resizeNeeded = true
		d.Reasons = append(d.Reasons, fmt.Sprintf("width %d exceeds max_width %d", a.Width, img.MaxWidth))
	}
	if img.MaxHeight > 0 && a.Height > img.MaxHeight {
		resizeNeeded = true
		d.Reasons = append(d.Reasons, fmt.Sprintf("height %d exceeds max_height %d", a.Height, img.MaxHeight))
	}

	// Size class.
	switch {
	case img.CriticalSizeBytes > 0 && a.SizeBytes >= img.CriticalSizeBytes:
		d.Class = models.ClassCritical
		d.Reasons = append(d.Reasons, fmt.Sprintf("size %s is at or above critical_size %s",
			FormatBytes(a.SizeBytes), FormatBytes(img.CriticalSizeBytes)))
	case img.WarningSizeBytes > 0 && a.SizeBytes >= img.WarningSizeBytes:
		d.Class = models.ClassHigh
		d.Reasons = append(d.Reasons, fmt.Sprintf("size %s is at or above warning_size %s",
			FormatBytes(a.SizeBytes), FormatBytes(img.WarningSizeBytes)))
	}

	// Traffic floors: this is what separates "oversized" from "important".
	switch {
	case img.MinDeliveredBytes > 0 && a.BytesDelivered < img.MinDeliveredBytes:
		d.Reasons = append(d.Reasons, fmt.Sprintf("delivered %s is below min_delivered_bytes %s",
			FormatBytes(a.BytesDelivered), FormatBytes(img.MinDeliveredBytes)))
		return d
	case img.MinRequests > 0 && a.Requests < img.MinRequests:
		d.Reasons = append(d.Reasons, fmt.Sprintf("%d requests is below min_requests %d", a.Requests, img.MinRequests))
		return d
	}

	// Target format: converting to a format the asset already is produces no
	// gain, only risk.
	target := normalizeTarget(img.TargetFormat)
	convertsFormat := target != "" && target != string(format)
	if !convertsFormat {
		d.Reasons = append(d.Reasons, fmt.Sprintf("already in the target format %s", format))
	} else {
		d.Reasons = append(d.Reasons, fmt.Sprintf("target format %s", target))
	}

	exceedsMaxBytes := img.MaxBytes > 0 && a.SizeBytes > img.MaxBytes
	needsWork := resizeNeeded || convertsFormat || exceedsMaxBytes || d.Class != ""
	if !needsWork {
		d.Action = ActionAlreadyOptimized
		d.Reasons = append(d.Reasons, "asset already meets the policy")
		return d
	}

	// Estimate the outcome for reporting. This is explicitly an estimate; the
	// optimizer replaces it with measured numbers when it actually encodes.
	if est, ok := EstimateEncodedSize(a, img); ok {
		d.EstimatedOptimizedSize = est
		if a.SizeBytes > 0 {
			d.EstimatedReductionPercent = float64(a.SizeBytes-est) / float64(a.SizeBytes) * 100
		}
	}
	if est := d.EstimatedOptimizedSize; est > 0 && a.SizeBytes > 0 && d.EstimatedReductionPercent < img.MinReductionPercent && !resizeNeeded {
		d.Action = ActionReview
		d.Reasons = append(d.Reasons, fmt.Sprintf("projected reduction %.1f%% is below min_reduction_percent %.1f%%",
			d.EstimatedReductionPercent, img.MinReductionPercent))
		return d
	}

	d.Action = ActionOptimize
	return d
}

// normalizeTarget maps a target format setting onto a canonical format.
func normalizeTarget(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return string(models.MediaFromContentType(s))
}

// formatAllowed reports whether the policy applies to a source format.
func formatAllowed(allowed []string, f models.MediaFormat) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if models.MediaFromContentType(a) == f {
			return true
		}
	}
	return false
}

// mediaPolicyAllowed reports whether the policy covers this media kind.
// v0.1.0 optimizes still images only; video is measured and reported, never
// transcoded.
func mediaPolicyAllowed(p models.OptimizationPolicy, f models.MediaFormat) bool {
	if f.Kind() == models.KindImage {
		return p.Images.WarningSizeBytes > 0 || p.Images.CriticalSizeBytes > 0 ||
			p.Images.TargetFormat != "" || p.Images.MaxWidth > 0 || p.Images.MaxHeight > 0 ||
			p.Images.MaxBytes > 0
	}
	return false
}

// ratioTable holds the expected size ratio for each source format, as
// observed across typical product-catalogue image sets.
//
// These numbers exist so that a scan can rank assets and project savings
// *before* anything is encoded. They are averages, therefore wrong for
// specific images, which is exactly why the optimizer recomputes them from real
// encodes and reports the difference. Ratios are deliberately conservative
// (they understate savings rather than overstate them).
//
// See docs/optimization.md for how to recalibrate these from your own corpus.
var ratioTable = map[models.MediaFormat]float64{
	models.FormatJPEG: 0.35, // JPEG -> WebP q80 typically lands near 30-40%
	models.FormatPNG:  0.45, // PNG -> WebP q80 lands near 35-55% for UI assets
	models.FormatWebP: 0.85, // WebP re-encode at equal quality is mostly lossless
}

// EstimateEncodedSize projects the encoded size of an asset under a policy.
//
// The projection applies two independent factors:
//
//	encoded = size * formatRatio * qualityFactor * resizeFactor
//
// where qualityFactor reflects the encoder's quality setting relative to the
// conservative reference used in ratioTable, and resizeFactor is the pixel-area
// ratio when the policy caps dimensions. Resizing dominates: cutting a 4K hero
// image to 1920 px wide removes roughly 80% of the pixels.
//
// ok is false when no defensible estimate can be made, for example because the
// dimensions are unknown while a resize is required. Callers must treat an
// unknown estimate as "no projection", never as zero savings.
func EstimateEncodedSize(a models.Asset, img models.ImagePolicy) (int64, bool) {
	if a.SizeBytes <= 0 {
		return 0, false
	}
	ratio, known := ratioTable[models.MediaFormat(a.Format)]
	if !known {
		return 0, false
	}

	quality := img.Quality
	if quality <= 0 {
		quality = 80
	}
	qualityFactor := math.Exp(0.045 * float64(quality-80))
	// Clamp so that an unusual quality setting cannot produce an absurd
	// projection: quality 1 is roughly 0.28x, quality 100 roughly 3.2x.
	qualityFactor = math.Max(0.25, math.Min(3.5, qualityFactor))

	factor := ratio * qualityFactor
	area, areaKnown := resizeAreaFactor(a, img)
	if !areaKnown {
		// A resize is required but the dimensions are unknown. The projected
		// size would then be meaningless, so refuse rather than guess.
		return 0, false
	}
	factor *= area

	estimated := int64(math.Round(float64(a.SizeBytes) * factor))
	if estimated < 1024 {
		// Below 1 KB an image is almost certainly an icon; the encoder's
		// container overhead makes the projection meaningless.
		estimated = 1024
	}
	if img.MaxBytes > 0 && estimated > img.MaxBytes {
		return 0, false
	}
	return estimated, true
}

// resizeAreaFactor returns the ratio of output pixel area to input pixel area
// after applying the policy's dimension caps, and whether that ratio is known.
//
// Encoded size scales with pixel count, not with width alone: halving both
// dimensions removes 75% of the pixels, not 50%. Using a width-only ratio would
// systematically overstate savings for large images, which is precisely the
// case Vyom exists to report.
//
// known is false when the policy caps dimensions and the asset's dimensions
// are unknown, because the caller then cannot distinguish "no resize" from
// "resize of unknown magnitude".
func resizeAreaFactor(a models.Asset, img models.ImagePolicy) (factor float64, known bool) {
	capsWidth := img.MaxWidth > 0
	capsHeight := img.MaxHeight > 0
	if !capsWidth && !capsHeight {
		return 1, true // no resizing configured
	}
	if a.Width <= 0 || a.Height <= 0 {
		return 0, false
	}

	w, h := float64(a.Width), float64(a.Height)
	if capsWidth && w > float64(img.MaxWidth) {
		// Preserve the aspect ratio when scaling to the width cap.
		h = h * float64(img.MaxWidth) / w
		w = float64(img.MaxWidth)
	}
	if capsHeight && h > float64(img.MaxHeight) {
		w = w * float64(img.MaxHeight) / h
		h = float64(img.MaxHeight)
	}
	return (w * h) / (float64(a.Width) * float64(a.Height)), true
}
