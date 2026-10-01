package policy

import (
	"math"

	"github.com/kaushalacts/vyom/pkg/models"
)

// CorpusTotals holds the aggregates every asset's impact is measured against.
//
// Impact is a relative measure by construction. An asset that delivered 4 GB is
// only interesting because the corpus delivered 30 TB and another asset
// delivered 3.8 TB. Computing impact requires the totals, which is why
// ComputeImpact takes them explicitly rather than reading global state.
type CorpusTotals struct {
	// BytesDelivered is total traffic across all matched assets.
	BytesDelivered int64
	// Requests is the total request count.
	Requests int64
	// PotentialSavings is total projected savings across all assets. It is
	// usually only known after the pass that computes per-asset estimates, so
	// it may be zero on the first pass; see ComputeImpactAll.
	PotentialSavings int64
	// Assets is the number of assets in the corpus.
	Assets int64
}

// IsEmpty reports whether the corpus has no measurable traffic.
func (t CorpusTotals) IsEmpty() bool { return t.BytesDelivered <= 0 || t.Assets <= 0 }

// Weights are the coefficients of the priority score. They sum to 1 so that
// PriorityScore is directly interpretable as a 0-100 rank.
//
// The weights encode the product decision that potential savings and delivered
// bytes dominate, and raw request counts only corroborate. Request count is the
// weakest signal on its own: a cached 5 KB icon requested 40 million times
// delivers a fraction of a terabyte and must not outrank a 1 MB hero image
// requested 3.8 million times.
type Weights struct {
	PotentialSavings float64 `json:"potential_savings"`
	DeliveredBytes   float64 `json:"delivered_bytes"`
	Requests         float64 `json:"requests"`
}

// DefaultWeights is Vyom's v0.1.0 weighting. Documented in
// docs/architecture.md under "Impact scoring"; overridable per call for teams
// with a different cost structure.
func DefaultWeights() Weights {
	return Weights{PotentialSavings: 0.50, DeliveredBytes: 0.30, Requests: 0.20}
}

// Validate rejects weights that do not form a convex combination.
func (w Weights) Validate() error {
	var errs models.ValidationErrors
	if w.PotentialSavings < 0 || w.DeliveredBytes < 0 || w.Requests < 0 {
		errs.Addf("weights", "must not be negative")
	}
	total := w.PotentialSavings + w.DeliveredBytes + w.Requests
	if math.Abs(total-1) > 1e-9 {
		errs.Addf("weights", "must sum to 1.0, got %v", total)
	}
	return errs.Err()
}

// ComputeImpact scores one asset against the corpus.
//
// The formulas, all documented in docs/architecture.md:
//
//	projected_bytes = current_bytes - potential_savings
//	savings_percent = potential_savings / current_bytes * 100
//
//	delivered_share = current_bytes / corpus_bytes
//	savings_share   = potential_savings / corpus_savings
//	request_share   = requests / corpus_requests
//
//	priority_score = 100 * ( w_savings * savings_share
//	                      + w_bytes   * delivered_share
//	                      + w_req     * request_share )
//
// Shares, not absolute values, are what make the score comparable across scans
// of different sizes: the same asset scores identically whether the corpus
// delivered 3 TB or 300 TB, and the top-ranked asset always scores near 100.
//
// The function is pure. It performs no I/O and does not look at global state,
// which is what makes the ranking reproducible and testable.
func ComputeImpact(a models.Asset, projectedSize int64, p models.OptimizationPolicy, totals CorpusTotals, w Weights) models.AssetImpact {
	img := p.Images
	imp := models.AssetImpact{
		AssetID:      a.ID,
		URL:          a.URL,
		StorageKey:   a.StorageKey,
		Requests:     a.Requests,
		CurrentBytes: a.BytesDelivered,
	}

	// Projected object size.
	//
	// A measured size always wins over a projected one: once the optimizer has
	// actually encoded the image, the measurement is ground truth. The
	// projection is used only while no measurement exists.
	est, ok := int64(0), false
	if projectedSize > 0 {
		est, ok = projectedSize, true
	} else {
		est, ok = EstimateEncodedSize(a, img)
	}
	if ok && est > 0 {
		imp.EstimatedOptimizedSize = est
	}

	// Traffic projection. Requests are held constant on purpose: optimizing an
	// image does not reduce how often it is requested, only how many bytes each
	// request costs. This is the single most common modelling error in tools
	// like this, and it is why Vyom's projections are conservative.
	scale := 1.0
	if ok && a.SizeBytes > 0 && est > 0 {
		scale = clamp(float64(est)/float64(a.SizeBytes), 0.05, 1.0)
	}
	imp.ProjectedBytes = int64(math.Round(float64(a.BytesDelivered) * scale))
	imp.PotentialSavings = a.BytesDelivered - imp.ProjectedBytes
	if imp.PotentialSavings < 0 {
		imp.PotentialSavings = 0
		imp.ProjectedBytes = a.BytesDelivered
	}
	if a.BytesDelivered > 0 {
		imp.SavingsPercent = float64(imp.PotentialSavings) / float64(a.BytesDelivered) * 100
	}

	if totals.BytesDelivered > 0 {
		imp.DeliveredShare = clamp(float64(a.BytesDelivered)/float64(totals.BytesDelivered), 0, 1)
	}
	if totals.Requests > 0 {
		imp.RequestShare = clamp(float64(a.Requests)/float64(totals.Requests), 0, 1)
	}
	if totals.PotentialSavings > 0 {
		imp.SavingsShare = clamp(float64(imp.PotentialSavings)/float64(totals.PotentialSavings), 0, 1)
	}
	imp.PriorityScore = PriorityScore(imp, totals, w)
	imp.Class = Classify(imp)
	imp.Reasons = Explain(a, img, imp)
	return imp
}

// PriorityScore turns shares into the 0-100 ranking value.
//
// When the corpus has no projected savings (for example when every asset is
// already optimized), the savings term would be zero for all rows and would
// silently halve every score. In that case Vyom redistributes its weight to the
// terms that are meaningful, so ranking order stays correct and the score stays
// comparable in magnitude.
func PriorityScore(imp models.AssetImpact, totals CorpusTotals, w Weights) float64 {
	ws, wb, wr := w.PotentialSavings, w.DeliveredBytes, w.Requests
	if totals.PotentialSavings <= 0 {
		// No savings information: renormalize over the remaining terms.
		remaining := wb + wr
		if remaining <= 0 {
			return 0
		}
		ws, wb, wr = 0, wb/remaining, wr/remaining
	}
	score := 100 * (ws*imp.SavingsShare + wb*imp.DeliveredShare + wr*imp.RequestShare)
	return clamp(score, 0, 100)
}

// Classify maps a score onto Vyom's severity vocabulary.
//
// The thresholds are cumulative shares of the corpus rather than absolute byte
// counts, so the classes mean the same thing in a 3 TB and a 300 TB account.
func Classify(imp models.AssetImpact) models.ImpactClass {
	switch {
	case imp.PotentialSavings <= 0:
		return models.ClassLow
	case imp.PriorityScore >= 50 || imp.DeliveredShare >= 0.10:
		return models.ClassCritical
	case imp.PriorityScore >= 20 || imp.DeliveredShare >= 0.02:
		return models.ClassHigh
	case imp.PriorityScore >= 5 || imp.DeliveredShare >= 0.005:
		return models.ClassMedium
	default:
		return models.ClassLow
	}
}

// Explain produces the plain-language reasons shown next to an asset.
//
// The wording is deliberately factual: Vyom reports what it measured, and never
// implies causation it cannot prove.
func Explain(a models.Asset, img models.ImagePolicy, imp models.AssetImpact) []string {
	var out []string
	if a.SizeBytes > 0 {
		dimensions := "unknown dimensions"
		if a.Width > 0 && a.Height > 0 {
			dimensions = FormatCount(int64(a.Width)) + "x" + FormatCount(int64(a.Height))
		}
		out = append(out, "object size "+FormatBytes(a.SizeBytes)+" at "+dimensions)
	}
	if a.Requests > 0 {
		out = append(out, "delivered "+FormatBytes(a.BytesDelivered)+" over "+FormatCount(a.Requests)+" requests")
	}
	if imp.EstimatedOptimizedSize > 0 {
		out = append(out, "projected optimized size "+FormatBytes(imp.EstimatedOptimizedSize)+
			" ("+FormatPercent(imp.SavingsPercent)+" reduction)")
	}
	if imp.DeliveredShare > 0 {
		out = append(out, "top contributor: "+FormatPercent(imp.DeliveredShare*100)+" of measured media traffic")
	}
	if img.MinimumSSIM > 0 {
		out = append(out, "candidates must reach SSIM >= "+FormatDecimal(img.MinimumSSIM, 2))
	}
	return out
}

// ComputeImpactAll scores a slice of assets in two passes.
//
// Two passes are necessary because PotentialSavings is itself a share of the
// corpus total, which is only known after every asset has been projected. A
// single pass would rank assets against a total that grows as it goes, and the
// first assets scanned would be systematically over-ranked.
//
// The slice is returned sorted by descending priority. For corpora that do not
// fit in memory, the caller should use RankStreaming instead.
func ComputeImpactAll(assets []models.Asset, p models.OptimizationPolicy, w Weights) []models.AssetImpact {
	totals := TotalsOfWithPolicy(assets, p)
	out := make([]models.AssetImpact, 0, len(assets))
	for _, a := range assets {
		out = append(out, ComputeImpact(a, 0, p, totals, w))
	}
	sortByPriority(out)
	return out
}

// TotalsOf computes the corpus totals for a slice of assets, including total
// projected savings.
//
// Projected savings are part of the totals because SavingsShare is defined
// against them. Leaving the field at zero would make every savings share zero
// and collapse the score onto the other two terms, silently changing the
// ranking that weights are supposed to control. The second pass projects each
// asset with the default weighting, which is exactly what ComputeImpactAll then
// does per asset.
func TotalsOf(assets []models.Asset) CorpusTotals {
	t := CorpusTotals{Assets: int64(len(assets))}
	for _, a := range assets {
		t.BytesDelivered += a.BytesDelivered
		t.Requests += a.Requests
	}
	var savings int64
	for _, a := range assets {
		if est, ok := EstimateEncodedSize(a, Default().Images); ok && a.SizeBytes > 0 {
			scale := clamp(float64(est)/float64(a.SizeBytes), 0.05, 1.0)
			savings += a.BytesDelivered - int64(math.Round(float64(a.BytesDelivered)*scale))
		}
	}
	t.PotentialSavings = savings
	return t
}

// TotalsOfWithPolicy is TotalsOf using a specific policy's projections. Use it
// whenever the caller has a policy in hand; TotalsOf exists for call sites that
// need corpus aggregates before a policy is chosen (for example a cost summary).
func TotalsOfWithPolicy(assets []models.Asset, p models.OptimizationPolicy) CorpusTotals {
	t := CorpusTotals{Assets: int64(len(assets))}
	for _, a := range assets {
		t.BytesDelivered += a.BytesDelivered
		t.Requests += a.Requests
	}
	var savings int64
	for _, a := range assets {
		if est, ok := EstimateEncodedSize(a, p.Images); ok && a.SizeBytes > 0 {
			scale := clamp(float64(est)/float64(a.SizeBytes), 0.05, 1.0)
			savings += a.BytesDelivered - int64(math.Round(float64(a.BytesDelivered)*scale))
		}
	}
	t.PotentialSavings = savings
	return t
}

// EstimatedSavingsOf sums the projected savings of a set of impacts.
func EstimatedSavingsOf(impacts []models.AssetImpact) int64 {
	var total int64
	for _, i := range impacts {
		total += i.PotentialSavings
	}
	return total
}

func sortByPriority(out []models.AssetImpact) {
	// Insertion sort keeps the dependency surface at zero and is stable, which
	// keeps report output reproducible for equal scores. AssetImpacts per scan
	// that reach this magnitude are already dominated by I/O, not sorting.
	for i := 1; i < len(out); i++ {
		cur := out[i]
		j := i - 1
		for j >= 0 && lessImpact(cur, out[j]) {
			out[j+1] = out[j]
			j--
		}
		out[j+1] = cur
	}
}

// lessImpact orders by priority, then by absolute savings, then by URL so that
// the ordering is total and reproducible.
func lessImpact(a, b models.AssetImpact) bool {
	if a.PriorityScore != b.PriorityScore {
		return a.PriorityScore > b.PriorityScore
	}
	if a.PotentialSavings != b.PotentialSavings {
		return a.PotentialSavings > b.PotentialSavings
	}
	return a.URL < b.URL
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
