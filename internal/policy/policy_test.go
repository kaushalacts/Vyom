package policy

import (
	"strings"
	"testing"

	"github.com/kaushalacts/vyom/pkg/models"
)

func TestDefaultPolicyIsValid(t *testing.T) {
	t.Parallel()
	if err := Validate(Default()); err != nil {
		t.Fatalf("built-in default policy must be valid: %v", err)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	base := func(mutate func(*models.OptimizationPolicy)) models.OptimizationPolicy {
		p := Default()
		if mutate != nil {
			mutate(&p)
		}
		return p
	}
	tests := []struct {
		name     string
		policy   models.OptimizationPolicy
		wantErrs []string
	}{
		{
			name:   "valid minimal",
			policy: base(func(p *models.OptimizationPolicy) { p.Images = models.ImagePolicy{TargetFormat: "webp", Quality: 80} }),
		},
		{
			name:     "missing name",
			policy:   base(func(p *models.OptimizationPolicy) { p.Name = "" }),
			wantErrs: []string{"policy.name"},
		},
		{
			name:     "missing version",
			policy:   base(func(p *models.OptimizationPolicy) { p.Version = "" }),
			wantErrs: []string{"policy.version"},
		},
		{
			name:     "quality out of range",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.Quality = 150 }),
			wantErrs: []string{"policy.images.quality"},
		},
		{
			name:     "ssim out of range",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.MinimumSSIM = 1.4 }),
			wantErrs: []string{"policy.images.minimum_ssim"},
		},
		{
			name:     "dangerously low ssim",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.MinimumSSIM = 0.5 }),
			wantErrs: []string{"below 0.8"},
		},
		{
			name:     "critical below warning",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.CriticalSizeBytes = 100 }),
			wantErrs: []string{"policy.images.critical_size"},
		},
		{
			name:     "unsupported target format",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.TargetFormat = "avif" }),
			wantErrs: []string{"policy.images.target_format"},
		},
		{
			name:     "unsupported allowed format",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.AllowedFormats = []string{"tiff"} }),
			wantErrs: []string{"policy.images.allowed_formats"},
		},
		{
			name:     "negative size",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.WarningSizeBytes = -1 }),
			wantErrs: []string{"policy.images.warning_size"},
		},
		{
			name:     "negative max bytes",
			policy:   base(func(p *models.OptimizationPolicy) { p.Images.MaxBytes = -5 }),
			wantErrs: []string{"policy.images.max_bytes"},
		},
		{
			name:     "negative pricing",
			policy:   base(func(p *models.OptimizationPolicy) { p.Pricing = models.PricingConfig{UnitPricePerGB: -1} }),
			wantErrs: []string{"policy.pricing.bandwidth_per_gb"},
		},
		{
			name: "multiple errors reported at once",
			policy: base(func(p *models.OptimizationPolicy) {
				p.Name = ""
				p.Version = ""
				p.Images.Quality = 0
				p.Images.Quality = 900
			}),
			wantErrs: []string{"policy.name", "policy.version", "policy.images.quality"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.policy)
			if len(tc.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("expected valid, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected errors containing %v", tc.wantErrs)
			}
			for _, want := range tc.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	t.Parallel()
	p := Default()

	tests := []struct {
		name       string
		asset      models.Asset
		wantAction Action
		wantClass  models.ImpactClass
		wantReason string
	}{
		{
			name:       "small low-traffic jpeg is ignored",
			asset:      models.Asset{Format: "jpeg", SizeBytes: 20_000, Requests: 10, BytesDelivered: 200_000},
			wantAction: ActionIgnore,
		},
		{
			name: "oversized but below traffic floor",
			asset: models.Asset{Format: "jpeg", SizeBytes: 800_000, Requests: 5,
				BytesDelivered: 4_000_000},
			wantAction: ActionIgnore,
			wantReason: "min_requests",
		},
		{
			name:       "oversized and heavily requested",
			asset:      models.Asset{Format: "jpeg", SizeBytes: 1_048_576, Requests: 3_800_000, BytesDelivered: 3_985_068_800},
			wantAction: ActionOptimize,
			wantClass:  models.ClassCritical,
			wantReason: "critical_size",
		},
		{
			name:       "resizable but small file",
			asset:      models.Asset{Format: "png", SizeBytes: 300_000, Width: 4000, Height: 3000, Requests: 500_000, BytesDelivered: 150_000_000_000},
			wantAction: ActionOptimize,
			wantReason: "max_width",
		},
		{
			name: "already webp, within policy",
			asset: models.Asset{Format: "webp", SizeBytes: 50_000, Width: 800, Height: 600,
				Requests: 1_000_000, BytesDelivered: 50_000_000_000},
			wantAction: ActionAlreadyOptimized,
			wantReason: "already meets the policy",
		},
		{
			name: "unsupported format",
			asset: models.Asset{Format: "heic", SizeBytes: 2_000_000, Requests: 9_000_000,
				BytesDelivered: 18_000_000_000_000},
			wantAction: ActionIgnore,
			wantReason: "not analyzable",
		},
		{
			name: "format not in allowed list",
			asset: models.Asset{Format: "png", SizeBytes: 900_000, Requests: 9_000_000,
				BytesDelivered: 8_100_000_000_000},
			wantAction: ActionIgnore,
			wantReason: "allowed_formats",
		},
	}
	policyWithAllowed := p
	policyWithAllowed.Images.AllowedFormats = []string{"jpeg"}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(p, tc.asset)
			if tc.name == "format not in allowed list" {
				got = Evaluate(policyWithAllowed, tc.asset)
			}
			if got.Action != tc.wantAction {
				t.Errorf("Action = %q, want %q (reasons: %v)", got.Action, tc.wantAction, got.Reasons)
			}
			if tc.wantClass != "" && got.Class != tc.wantClass {
				t.Errorf("Class = %q, want %q", got.Class, tc.wantClass)
			}
			if tc.wantReason != "" && !containsReason(got.Reasons, tc.wantReason) {
				t.Errorf("reasons %v do not mention %q", got.Reasons, tc.wantReason)
			}
			if len(got.Reasons) == 0 {
				t.Error("every decision must explain itself")
			}
		})
	}

	t.Run("allowed formats is respected", func(t *testing.T) {
		got := Evaluate(policyWithAllowed, models.Asset{Format: "png", SizeBytes: 900_000,
			Requests: 9_000_000, BytesDelivered: 8_100_000_000_000})
		if got.Action != ActionIgnore {
			t.Errorf("png must be excluded when only jpeg is allowed, got %q", got.Action)
		}
	})
}

func TestEvaluateNeverOptimizesOnSizeAlone(t *testing.T) {
	t.Parallel()
	// This is the product's central claim, so it is pinned as a test: a large
	// asset with negligible traffic must never become an opportunity.
	p := Default()
	smallButHuge := models.Asset{Format: "jpeg", SizeBytes: 900_000, Requests: 10,
		BytesDelivered: 9_000_000}
	if d := Evaluate(p, smallButHuge); d.Opportunity() {
		t.Fatalf("a 900 KB asset with 10 requests must not be an opportunity: %+v", d)
	}

	bigAndFrequent := smallButHuge
	bigAndFrequent.Requests = 5_000_000
	bigAndFrequent.BytesDelivered = 4_500_000_000_000
	if d := Evaluate(p, bigAndFrequent); !d.Opportunity() {
		t.Fatalf("the same asset at 5M requests must be an opportunity: %+v", d)
	}
}

func containsReason(reasons []string, substr string) bool {
	for _, r := range reasons {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

func TestSetLookup(t *testing.T) {
	t.Parallel()
	s := Set{Name: "policies", Policies: []models.OptimizationPolicy{
		{Name: "default"}, {Name: "ecommerce"},
	}}
	if p, ok := s.Lookup("ecommerce"); !ok || p.Name != "ecommerce" {
		t.Errorf("Lookup(ecommerce) = %+v, %v", p, ok)
	}
	if _, ok := s.Lookup("ECOMMERCE"); !ok {
		t.Error("Lookup must be case-insensitive")
	}
	if _, ok := s.Lookup("nope"); ok {
		t.Error("unknown policy must not be found")
	}
	if p, ok := s.Lookup(""); !ok || p.Name != "default" {
		t.Errorf("empty name must resolve to the first policy, got %+v", p)
	}
	if got := s.Names(); len(got) != 2 || got[0] != "default" {
		t.Errorf("Names() = %v", got)
	}
	empty := Set{}
	if p, ok := empty.Lookup(""); !ok || p.Name != DefaultName {
		t.Error("empty set must fall back to the built-in default")
	}
}
