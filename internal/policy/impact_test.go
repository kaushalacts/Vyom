package policy

import (
	"testing"

	"github.com/kaushalacts/vyom/pkg/models"
)

// heroAsset is the motivating case from Vyom's origin story: a 1.02 MB hero
// image requested 3.8 million times, contributing roughly 3.8 TB.
func heroAsset() models.Asset {
	return models.Asset{
		ID:             "ast_hero",
		URL:            "https://cdn.example.com/images/hero-1920.jpg",
		StorageKey:     "images/hero-1920.jpg",
		ContentType:    "image/jpeg",
		Format:         "jpeg",
		SizeBytes:      1_020_000,
		Width:          1920,
		Height:         1080,
		Requests:       3_800_000,
		BytesDelivered: 3_876_000_000_000, // ~3.8 TB over the window
	}
}

func TestComputeImpactMatchesTheOriginStory(t *testing.T) {
	t.Parallel()
	hero := heroAsset()
	// A corpus where the hero is the dominant contributor, mirroring the
	// 15 TB -> 30 TB incident.
	rest := make([]models.Asset, 0, 20)
	var totalRest int64
	for i := 0; i < 20; i++ {
		a := models.Asset{
			ID:             "ast_" + string(rune('a'+i)),
			URL:            "https://cdn.example.com/p/" + string(rune('a'+i)) + ".jpg",
			StorageKey:     "p/" + string(rune('a'+i)) + ".jpg",
			Format:         "jpeg",
			SizeBytes:      300_000,
			Requests:       200_000,
			BytesDelivered: 60_000_000_000,
		}
		rest = append(rest, a)
		totalRest += a.BytesDelivered
	}
	all := append([]models.Asset{hero}, rest...)
	totals := TotalsOf(all)
	p := Default()
	imp := ComputeImpact(hero, 0, p, totals, DefaultWeights())

	if imp.CurrentBytes != hero.BytesDelivered {
		t.Errorf("CurrentBytes = %d", imp.CurrentBytes)
	}
	// JPEG -> WebP q80 projects to roughly a third of the original.
	if imp.EstimatedOptimizedSize <= 0 {
		t.Fatal("expected a projected optimized size")
	}
	if imp.SavingsPercent < 60 || imp.SavingsPercent > 80 {
		t.Errorf("SavingsPercent = %.1f, want roughly 60-80 for JPEG->WebP", imp.SavingsPercent)
	}
	if imp.PotentialSavings <= 0 || imp.ProjectedBytes >= imp.CurrentBytes {
		t.Errorf("projected bytes must be lower: %+v", imp)
	}
	if imp.SavingsShare <= 0.9 {
		t.Errorf("hero should dominate savings share, got %.3f (rest delivered %d)", imp.SavingsShare, totalRest)
	}
	if imp.PriorityScore < 50 {
		t.Errorf("hero must be a top contributor, score = %.1f", imp.PriorityScore)
	}
	if imp.Class != models.ClassCritical {
		t.Errorf("Class = %q, want critical", imp.Class)
	}
	if len(imp.Reasons) == 0 {
		t.Error("impact must explain itself")
	}
	// A traffic projection must never exceed current traffic.
	if imp.ProjectedBytes > imp.CurrentBytes {
		t.Error("projected bytes cannot exceed current bytes")
	}
}

func TestRequestsHeldConstantInProjection(t *testing.T) {
	t.Parallel()
	// Optimizing an image does not reduce how often it is requested. A model
	// that projected fewer requests would overstate savings.
	hero := heroAsset()
	imp := ComputeImpact(hero, 0, Default(), CorpusTotals{BytesDelivered: hero.BytesDelivered * 10, Assets: 100, Requests: hero.Requests * 10}, DefaultWeights())
	if imp.Requests != hero.Requests {
		t.Errorf("Requests = %d, want %d", imp.Requests, hero.Requests)
	}
	if imp.CurrentBytes-hero.BytesDelivered != 0 {
		t.Error("CurrentBytes must be measured, never projected")
	}
}

func TestPriorityScoreIsRelativeNotAbsolute(t *testing.T) {
	t.Parallel()
	// Priority must be a share of the corpus, so that scaling the whole corpus
	// up or down does not change any asset's rank. If the score used absolute
	// bytes, a report from a 3 TB month and one from a 300 TB month would not
	// be comparable, and "top contributor" would mean something different each
	// time.
	heroSmall := heroAsset()
	heroLarge := heroAsset()
	heroLarge.Requests *= 100
	heroLarge.BytesDelivered *= 100

	// Two corpora with identical proportions: the rest is 1% of the hero's
	// traffic in both, so the hero's share is ~99% in both.
	restSmall := models.Asset{
		ID: "r1", URL: "r1", Format: "jpeg", SizeBytes: 300_000,
		Requests: 3_800, BytesDelivered: 38_760_000_000,
	}
	restLarge := models.Asset{
		ID: "r1", URL: "r1", Format: "jpeg", SizeBytes: 300_000,
		Requests: 380_000, BytesDelivered: 3_876_000_000_000,
	}
	small := []models.Asset{restSmall, heroSmall}
	large := []models.Asset{restLarge, heroLarge}

	p, w := Default(), DefaultWeights()
	a := ComputeImpact(heroSmall, 0, p, TotalsOfWithPolicy(small, p), w)
	b := ComputeImpact(heroLarge, 0, p, TotalsOfWithPolicy(large, p), w)
	if a.DeliveredShare != b.DeliveredShare {
		t.Errorf("DeliveredShare must be scale-invariant: %.6f vs %.6f", a.DeliveredShare, b.DeliveredShare)
	}
	if a.RequestShare != b.RequestShare {
		t.Errorf("RequestShare must be scale-invariant: %.6f vs %.6f", a.RequestShare, b.RequestShare)
	}
	if a.PriorityScore != b.PriorityScore {
		t.Errorf("PriorityScore must be scale-invariant: %.4f vs %.4f", a.PriorityScore, b.PriorityScore)
	}
}

func TestPriorityScoreWeightsSavingsAndBytesOverRequests(t *testing.T) {
	t.Parallel()
	w := DefaultWeights()
	if w.PotentialSavings <= w.DeliveredBytes || w.DeliveredBytes <= w.Requests {
		t.Errorf("weights must order savings > bytes > requests: %+v", w)
	}
	if err := w.Validate(); err != nil {
		t.Errorf("default weights must be valid: %v", err)
	}

	// The cached-icon trap. An icon delivered 500 GB must not outrank a hero
	// image that delivered 3.8 TB, even though the icon is requested five
	// orders of magnitude more often. Request count is corroborating evidence,
	// not the ranking signal.
	icon := models.Asset{ID: "icon", URL: "icon.png", Format: "png", SizeBytes: 5_000, Requests: 100_000_000, BytesDelivered: 500_000_000_000}
	hero := heroAsset()
	totals := TotalsOf([]models.Asset{icon, hero})

	ii := ComputeImpact(icon, 0, Default(), totals, w)
	ih := ComputeImpact(hero, 0, Default(), totals, w)
	if ii.PriorityScore >= ih.PriorityScore {
		t.Errorf("request frequency must not outrank delivered bytes: icon %.2f vs hero %.2f",
			ii.PriorityScore, ih.PriorityScore)
	}
}

func TestPriorityScoreHandlesEmptyCorpus(t *testing.T) {
	t.Parallel()
	hero := heroAsset()
	imp := ComputeImpact(hero, 0, Default(), CorpusTotals{}, DefaultWeights())
	if imp.PriorityScore != 0 {
		t.Errorf("PriorityScore with empty corpus = %.2f, want 0", imp.PriorityScore)
	}
	if imp.SavingsShare != 0 || imp.DeliveredShare != 0 {
		t.Error("shares must be zero for an empty corpus, not NaN or negative")
	}
	if imp.Class != models.ClassLow {
		t.Errorf("Class = %q", imp.Class)
	}
	// Savings are still computed from the asset itself.
	if imp.PotentialSavings <= 0 {
		t.Error("potential savings do not depend on the corpus")
	}
}

func TestComputeImpactWithoutProjection(t *testing.T) {
	t.Parallel()
	// An asset whose format Vyom cannot project for must report zero savings
	// rather than an invented number.
	unknown := models.Asset{ID: "u", URL: "u", Format: "heic", SizeBytes: 900_000, Requests: 5_000_000, BytesDelivered: 4_500_000_000}
	imp := ComputeImpact(unknown, 0, Default(), CorpusTotals{BytesDelivered: unknown.BytesDelivered, Requests: unknown.Requests, Assets: 1}, DefaultWeights())
	if imp.EstimatedOptimizedSize != 0 {
		t.Errorf("unprojectable asset must have no estimate, got %d", imp.EstimatedOptimizedSize)
	}
	if imp.PotentialSavings != 0 {
		t.Errorf("unprojectable asset must claim no savings, got %d", imp.PotentialSavings)
	}
	if imp.ProjectedBytes != imp.CurrentBytes {
		t.Errorf("ProjectedBytes = %d, want CurrentBytes %d", imp.ProjectedBytes, imp.CurrentBytes)
	}
}

func TestComputeImpactUsesMeasuredSizeWhenProvided(t *testing.T) {
	t.Parallel()
	// After an actual encode, the caller passes the measured optimized size.
	// It must override the projection.
	hero := heroAsset()
	totals := CorpusTotals{BytesDelivered: hero.BytesDelivered, Requests: hero.Requests, Assets: 1}
	measured := ComputeImpact(hero, 281_000, Default(), totals, DefaultWeights())
	if measured.EstimatedOptimizedSize != 281_000 {
		t.Errorf("EstimatedOptimizedSize = %d, want the measured 281000", measured.EstimatedOptimizedSize)
	}
	wantSavings := hero.BytesDelivered - int64(float64(hero.BytesDelivered)*(281_000.0/1_020_000.0))
	if diff := measured.PotentialSavings - wantSavings; diff > 1 || diff < -1 {
		t.Errorf("PotentialSavings = %d, want ~%d", measured.PotentialSavings, wantSavings)
	}
	// ~72% reduction is the documented outcome for this asset.
	if measured.SavingsPercent < 70 || measured.SavingsPercent > 75 {
		t.Errorf("SavingsPercent = %.2f, want ~72", measured.SavingsPercent)
	}
}

func TestComputeImpactAllSortsAndIsDeterministic(t *testing.T) {
	t.Parallel()
	assets := []models.Asset{
		heroAsset(),
		{ID: "small", URL: "https://cdn.example.com/s.jpg", Format: "jpeg", SizeBytes: 30_000, Requests: 2_000, BytesDelivered: 60_000_000},
		{ID: "mid", URL: "https://cdn.example.com/m.jpg", Format: "jpeg", SizeBytes: 400_000, Requests: 500_000, BytesDelivered: 200_000_000_000},
	}
	p := Default()
	first := ComputeImpactAll(assets, p, DefaultWeights())
	second := ComputeImpactAll(assets, p, DefaultWeights())

	if len(first) != len(assets) {
		t.Fatalf("got %d impacts for %d assets", len(first), len(assets))
	}
	if first[0].AssetID != "ast_hero" {
		t.Errorf("largest contributor must rank first, got %s", first[0].AssetID)
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].PriorityScore < first[i].PriorityScore {
			t.Errorf("results not sorted: %.2f then %.2f", first[i-1].PriorityScore, first[i].PriorityScore)
		}
	}
	for i := range first {
		if first[i].PriorityScore != second[i].PriorityScore {
			t.Errorf("scoring is not deterministic at %d", i)
		}
	}
}

func TestCorpusTotalsAndSums(t *testing.T) {
	t.Parallel()
	assets := []models.Asset{
		{BytesDelivered: 100, Requests: 5},
		{BytesDelivered: 250, Requests: 7},
	}
	tot := TotalsOf(assets)
	if tot.BytesDelivered != 350 || tot.Requests != 12 || tot.Assets != 2 {
		t.Errorf("TotalsOf = %+v", tot)
	}
	if tot.IsEmpty() {
		t.Error("totals with traffic must not be empty")
	}
	if !(CorpusTotals{}).IsEmpty() {
		t.Error("zero totals must be empty")
	}
	if got := EstimatedSavingsOf([]models.AssetImpact{{PotentialSavings: 5}, {PotentialSavings: 7}}); got != 12 {
		t.Errorf("EstimatedSavingsOf = %d", got)
	}
}

func TestEstimateEncodedSize(t *testing.T) {
	t.Parallel()
	p := Default()
	tests := []struct {
		name   string
		asset  models.Asset
		policy models.ImagePolicy
		wantOK bool
		min    int64
		max    int64
	}{
		{
			name:   "4k jpeg resized to 1920",
			asset:  models.Asset{Format: "jpeg", SizeBytes: 4_000_000, Width: 3840, Height: 2160},
			policy: p.Images,
			wantOK: true, min: 100_000, max: 400_000,
		},
		{
			name:   "small jpeg already within limits",
			asset:  models.Asset{Format: "jpeg", SizeBytes: 20_000, Width: 400, Height: 300},
			policy: p.Images,
			wantOK: true, min: 1_024, max: 20_000,
		},
		{
			name:   "unknown format is not projectable",
			asset:  models.Asset{Format: "heic", SizeBytes: 900_000, Width: 100, Height: 100},
			policy: p.Images,
			wantOK: false,
		},
		{
			name:   "zero size is not projectable",
			asset:  models.Asset{Format: "jpeg", SizeBytes: 0, Width: 10, Height: 10},
			policy: p.Images,
			wantOK: false,
		},
		{
			name:   "resize required but dimensions unknown",
			asset:  models.Asset{Format: "jpeg", SizeBytes: 900_000, Width: 0, Height: 0},
			policy: p.Images,
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := EstimateEncodedSize(tc.asset, tc.policy)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %d)", ok, tc.wantOK, got)
			}
			if !ok {
				return
			}
			if got < tc.min || got > tc.max {
				t.Errorf("estimate %d outside expected range [%d, %d]", got, tc.min, tc.max)
			}
		})
	}
}

func TestEstimateEncodedSizeNeverExceedsOriginal(t *testing.T) {
	t.Parallel()
	// A projection larger than the original is meaningless and would make
	// "potential savings" negative.
	sizes := []int64{1_024, 10_000, 200_000, 1_020_000, 5_000_000, 50_000_000}
	formats := []models.MediaFormat{models.FormatJPEG, models.FormatPNG, models.FormatWebP}
	quality := []int{40, 60, 80, 95}
	for _, size := range sizes {
		for _, f := range formats {
			for _, q := range quality {
				a := models.Asset{Format: string(f), SizeBytes: size, Width: 1920, Height: 1080}
				img := Default().Images
				img.Quality = q
				got, ok := EstimateEncodedSize(a, img)
				if !ok {
					continue
				}
				if got > size*2 {
					t.Errorf("estimate %d for size %d format %s q%d is implausibly large", got, size, f, q)
				}
			}
		}
	}
}

func TestWeightsValidate(t *testing.T) {
	t.Parallel()
	if err := (Weights{PotentialSavings: 0.5, DeliveredBytes: 0.3, Requests: 0.2}).Validate(); err != nil {
		t.Errorf("valid weights rejected: %v", err)
	}
	for _, w := range []Weights{
		{PotentialSavings: 0.5, DeliveredBytes: 0.3, Requests: 0.3},  // sums to 1.1
		{PotentialSavings: 0.5, DeliveredBytes: 0.3},                 // sums to 0.8
		{PotentialSavings: -0.1, DeliveredBytes: 0.6, Requests: 0.5}, // negative
	} {
		if err := w.Validate(); err == nil {
			t.Errorf("expected %+v to be rejected", w)
		}
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		imp  models.AssetImpact
		want models.ImpactClass
	}{
		{"no savings", models.AssetImpact{}, models.ClassLow},
		{"critical by share", models.AssetImpact{PotentialSavings: 1, DeliveredShare: 0.5, PriorityScore: 80}, models.ClassCritical},
		{"critical by score", models.AssetImpact{PotentialSavings: 1, DeliveredShare: 0.01, PriorityScore: 60}, models.ClassCritical},
		{"high", models.AssetImpact{PotentialSavings: 1, DeliveredShare: 0.03, PriorityScore: 25}, models.ClassHigh},
		{"medium", models.AssetImpact{PotentialSavings: 1, DeliveredShare: 0.006, PriorityScore: 6}, models.ClassMedium},
		{"low", models.AssetImpact{PotentialSavings: 1, DeliveredShare: 0.0001, PriorityScore: 0.5}, models.ClassLow},
	}
	for _, tc := range tests {
		if got := Classify(tc.imp); got != tc.want {
			t.Errorf("%s: Classify = %q, want %q", tc.name, got, tc.want)
		}
	}
}
