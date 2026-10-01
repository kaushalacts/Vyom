package models

import "time"

// AssetImpact is the ranked, relative importance of one asset.
//
// Impact is intentionally *relative to the scanned corpus*: a 900 KB image
// requested ten times is unremarkable next to a 900 KB image requested five
// million times, and a fixed threshold can never express that. Every field is
// derived from observed data plus a policy, never from assumptions about cost.
type AssetImpact struct {
	// ID is Vyom's stable identifier for the impact (hex, 32 chars).
	ID string `json:"id"`
	// AssetID references the asset.
	AssetID string `json:"asset_id"`
	// URL is denormalized so that ranked lists render without a join.
	URL string `json:"url"`
	// StorageKey is the storage location of the asset.
	StorageKey string `json:"storage_key,omitempty"`
	// Requests observed in the scan window.
	Requests int64 `json:"requests"`
	// CurrentBytes is the bytes the asset delivered in the window.
	CurrentBytes int64 `json:"current_bytes"`
	// ProjectedBytes is the estimated traffic after optimization.
	ProjectedBytes int64 `json:"projected_bytes"`
	// PotentialSavings is CurrentBytes - ProjectedBytes. It is a projection
	// based on a policy and an estimated encoded size, not a guarantee.
	PotentialSavings int64 `json:"potential_savings"`
	// SavingsPercent is PotentialSavings as a percentage of CurrentBytes.
	SavingsPercent float64 `json:"savings_percent"`
	// PriorityScore ranks the asset within its scan, 0-100, where 100 is the
	// single largest contributor. See internal/analyzer for the formula.
	PriorityScore float64 `json:"priority_score"`
	// DeliveredShare is CurrentBytes as a fraction (0-1) of corpus traffic.
	DeliveredShare float64 `json:"delivered_share"`
	// SavingsShare is PotentialSavings as a fraction (0-1) of corpus savings.
	SavingsShare float64 `json:"savings_share"`
	// RequestShare is Requests as a fraction (0-1) of corpus requests.
	RequestShare float64 `json:"request_share"`
	// EstimatedOptimizedSize is the per-object projected size.
	EstimatedOptimizedSize int64 `json:"estimated_optimized_size"`
	// Class is the human-readable bucket used by reports and the dashboard.
	Class ImpactClass `json:"class"`
	// Reasons explains, in plain language, why the asset scored as it did.
	Reasons []string `json:"reasons,omitempty"`
	// ScanID is the scan that produced the impact.
	ScanID string `json:"scan_id,omitempty"`
}

// ImpactClass is Vyom's severity vocabulary for an asset.
//
// Vyom deliberately avoids the word "waste" in technical output: an asset is
// only ever "oversized" or an "optimization opportunity".
type ImpactClass string

// Impact classes.
const (
	// ClassLow: delivered traffic is small relative to the corpus.
	ClassLow ImpactClass = "low"
	// ClassMedium: worth reviewing during normal maintenance.
	ClassMedium ImpactClass = "medium"
	// ClassHigh: a top contributor; should be addressed deliberately.
	ClassHigh ImpactClass = "high"
	// ClassCritical: single largest contributors to traffic.
	ClassCritical ImpactClass = "critical"
)

// Rank returns a sortable integer for the class.
func (c ImpactClass) Rank() int {
	switch c {
	case ClassCritical:
		return 3
	case ClassHigh:
		return 2
	case ClassMedium:
		return 1
	default:
		return 0
	}
}

// IsOpportunity reports whether the class is worth acting on.
func (c ImpactClass) IsOpportunity() bool { return c.Rank() >= ClassMedium.Rank() }

// TimeWindow is a half-open interval [Start, End) in UTC used throughout Vyom.
type TimeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NewTimeWindow builds a normalized UTC window.
func NewTimeWindow(start, end time.Time) TimeWindow {
	return TimeWindow{Start: start.UTC(), End: end.UTC()}
}

// Validate reports whether the window is usable.
func (w TimeWindow) Validate() error {
	switch {
	case w.Start.IsZero() || w.End.IsZero():
		return errValidation("time window: start and end are required")
	case w.End.Before(w.Start):
		return errValidation("time window: end is before start")
	case w.End.Equal(w.Start):
		return errValidation("time window: start and end are identical")
	default:
		return nil
	}
}

// Duration returns the window length.
func (w TimeWindow) Duration() time.Duration { return w.End.Sub(w.Start) }

// Contains reports whether t falls inside the window.
func (w TimeWindow) Contains(t time.Time) bool {
	if t.IsZero() {
		return false
	}
	t = t.UTC()
	return !t.Before(w.Start) && t.Before(w.End)
}

// String renders the window as "2006-01-02 → 2006-01-02".
func (w TimeWindow) String() string {
	return w.Start.Format("2006-01-02") + " → " + w.End.Format("2006-01-02")
}

// CostEstimate is a clearly-labelled estimate of cost, never an invoice.
//
// Vyom cannot know a CDN's actual billing. Every estimate is derived from
// user-supplied unit prices and must be presented as an estimate.
type CostEstimate struct {
	Currency string `json:"currency"`
	// Bytes the estimate was computed from.
	Bytes int64 `json:"bytes"`
	// UnitPrice is the configured price per GB (decimal).
	UnitPricePerGB float64 `json:"unit_price_per_gb"`
	// Amount is the estimated cost for Bytes.
	Amount float64 `json:"amount"`
	// PricingConfigured is false when no pricing was supplied; Amount is then
	// meaningless and must not be displayed.
	PricingConfigured bool `json:"pricing_configured"`
	// Basis explains the period the estimate covers, e.g. "2026-09-01 → 2026-09-30".
	Basis string `json:"basis,omitempty"`
	// Disclaimer is the human-readable label required by the product spec.
	Disclaimer string `json:"disclaimer,omitempty"`
}

// Disclaimers used across the product.
const (
	// DisclaimerCost marks any monetary figure as an estimate.
	DisclaimerCost = "Estimated value based on user-supplied unit pricing. Not a CDN invoice."
	// DisclaimerSavings marks any savings figure as a projection.
	DisclaimerSavings = "Projected from a policy and an estimated encoded size. Actual results depend on the encoder and on how the asset is re-published."
	// DisclaimerCausality marks a contributor analysis as correlational.
	DisclaimerCausality = "Contribution analysis is correlational: it ranks observed traffic, it does not establish root cause."
)
