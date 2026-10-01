package models

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAssetJSONContract(t *testing.T) {
	t.Parallel()
	// The dashboard and any third-party integration (for example the future
	// GitHub Action) depend on these field names. Changing them is a breaking
	// API change, so the contract is pinned by this test.
	var a Asset
	in := `{
		"id":"ast_1","url":"https://cdn.example.com/a.jpg","storage_key":"a.jpg",
		"content_type":"image/jpeg","format":"jpeg","size_bytes":1024000,
		"width":1920,"height":1080,"requests":3800000,"bytes_delivered":3872000000,
		"first_seen":"2026-09-01T00:00:00Z","last_seen":"2026-09-30T00:00:00Z"
	}`
	if err := json.Unmarshal([]byte(in), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ID != "ast_1" || a.BytesDelivered != 3_872_000_000 || a.Format != "jpeg" {
		t.Fatalf("unexpected decode: %+v", a)
	}
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{
		`"id"`, `"url"`, `"storage_key"`, `"size_bytes"`, `"width"`, `"height"`,
		`"requests"`, `"bytes_delivered"`, `"first_seen"`, `"last_seen"`, `"content_type"`, `"format"`,
	} {
		if !strings.Contains(string(out), field) {
			t.Errorf("marshalled asset missing %s: %s", field, out)
		}
	}
}

func TestOptimizedAssetNeverSerializesBytes(t *testing.T) {
	t.Parallel()
	oa := OptimizedAsset{StorageKey: "a.webp", Data: []byte("binary"), ContentType: "image/webp"}
	out, err := json.Marshal(oa)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "binary") {
		t.Errorf("OptimizedAsset.Data must never be serialized: %s", out)
	}
	if !strings.Contains(string(out), `"storage_key":"a.webp"`) {
		t.Errorf("unexpected payload: %s", out)
	}
}

func TestTrafficRecordValidate(t *testing.T) {
	t.Parallel()
	ok := TrafficRecord{URL: "https://x/a.jpg", Requests: 1, BytesDelivered: 2,
		StartTime: time.Now().Add(-time.Hour), EndTime: time.Now()}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	tests := []struct {
		name string
		rec  TrafficRecord
	}{
		{"no url", TrafficRecord{}},
		{"negative requests", TrafficRecord{URL: "u", Requests: -1}},
		{"negative bytes", TrafficRecord{URL: "u", BytesDelivered: -1}},
		{"end before start", TrafficRecord{URL: "u", StartTime: time.Now(), EndTime: time.Now().Add(-time.Hour)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.rec.Validate(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestObjectMetadataValidate(t *testing.T) {
	t.Parallel()
	if err := (ObjectMetadata{Key: "a/b.jpg", SizeBytes: 10}).Validate(); err != nil {
		t.Fatalf("valid object rejected: %v", err)
	}
	if err := (ObjectMetadata{Key: ""}).Validate(); err == nil {
		t.Error("empty key must be rejected")
	}
	if err := (ObjectMetadata{Key: "a", SizeBytes: -5}).Validate(); err == nil {
		t.Error("negative size must be rejected")
	}
}

func TestTimeWindow(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	w := NewTimeWindow(start, end)
	if err := w.Validate(); err != nil {
		t.Fatalf("valid window rejected: %v", err)
	}
	if got := w.Duration(); got != 30*24*time.Hour {
		t.Errorf("Duration() = %v", got)
	}
	if !w.Contains(start) {
		t.Error("window must contain its start")
	}
	if w.Contains(end) {
		t.Error("window is half-open; end must be excluded")
	}
	if w.Contains(time.Time{}) {
		t.Error("zero time must not be contained")
	}
	if got := w.String(); got != "2026-09-01 → 2026-10-01" {
		t.Errorf("String() = %q", got)
	}
	for _, bad := range []TimeWindow{
		{},
		{Start: end, End: start},
		{Start: start, End: start},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("expected %+v to be invalid", bad)
		}
	}
}

func TestScanStatusSemantics(t *testing.T) {
	t.Parallel()
	terminal := []ScanStatus{ScanStatusCompleted, ScanStatusPartial, ScanStatusFailed, ScanStatusCancelled}
	for _, s := range terminal {
		if !s.Terminal() || !s.Valid() {
			t.Errorf("%q should be terminal and valid", s)
		}
	}
	for _, s := range []ScanStatus{ScanStatusPending, ScanStatusRunning} {
		if s.Terminal() {
			t.Errorf("%q must not be terminal", s)
		}
	}
	if ScanStatus("weird").Valid() {
		t.Error("unknown status must not be valid")
	}
	if got := ScanStages(); len(got) != 9 || got[0] != StageValidate {
		t.Errorf("unexpected stage list: %v", got)
	}
}

func TestImpactClassRanking(t *testing.T) {
	t.Parallel()
	order := []ImpactClass{ClassLow, ClassMedium, ClassHigh, ClassCritical}
	for i := 1; i < len(order); i++ {
		if order[i].Rank() <= order[i-1].Rank() {
			t.Errorf("%q must rank above %q", order[i], order[i-1])
		}
	}
	if ClassLow.IsOpportunity() {
		t.Error("low impact must not be an opportunity")
	}
	for _, c := range []ImpactClass{ClassMedium, ClassHigh, ClassCritical} {
		if !c.IsOpportunity() {
			t.Errorf("%q should be an opportunity", c)
		}
	}
}

func TestPageRequestNormalizeAndValidate(t *testing.T) {
	t.Parallel()
	got := PageRequest{Limit: 10000, Offset: -5, Order: "asc"}.Normalize()
	if got.Limit != MaxPageLimit || got.Offset != 0 {
		t.Errorf("Normalize() = %+v", got)
	}
	if def := (PageRequest{}).Normalize(); def.Limit != DefaultPageLimit || def.Order != SortDesc {
		t.Errorf("default normalization = %+v", def)
	}
	if err := (PageRequest{Limit: -1}).Validate(); err == nil {
		t.Error("negative limit must be rejected")
	}
	if err := (PageRequest{Sort: "secret_column"}).Validate("bytes_delivered", "requests"); err == nil {
		t.Error("unlisted sort column must be rejected (prevents SQL injection via ORDER BY)")
	}
	if err := (PageRequest{Sort: "bytes_delivered"}).Validate("bytes_delivered"); err != nil {
		t.Errorf("listed sort column rejected: %v", err)
	}
	if err := (PageRequest{Order: "sideways"}).Validate(); err == nil {
		t.Error("invalid order must be rejected")
	}
}

func TestNewPageInfo(t *testing.T) {
	t.Parallel()
	info := NewPageInfo(PageRequest{Limit: 10, Offset: 0}, 10, 35)
	if !info.HasMore || info.NextOffset != 10 || info.Total != 35 {
		t.Errorf("unexpected page info: %+v", info)
	}
	last := NewPageInfo(PageRequest{Limit: 10, Offset: 30}, 5, 35)
	if last.HasMore {
		t.Error("last page must not advertise more data")
	}
}

func TestValidationErrorsAggregate(t *testing.T) {
	t.Parallel()
	var errs ValidationErrors
	if errs.Err() != nil {
		t.Error("empty aggregate must produce nil")
	}
	errs.Addf("a", "bad %d", 1)
	errs.Add(errors.New("plain"))
	errs.Add(nil)
	err := errs.Err()
	if err == nil || !strings.Contains(err.Error(), "a: bad 1") || !strings.Contains(err.Error(), "plain") {
		t.Errorf("aggregate error = %v", err)
	}
	if !IsValidationError(err) {
		t.Error("aggregate must be a validation error")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "a" {
		t.Errorf("expected field a, got %+v", ve)
	}
}

func TestNewIDShape(t *testing.T) {
	t.Parallel()
	a, b := NewID(PrefixAsset), NewID(PrefixAsset)
	if a == b {
		t.Error("IDs must be unique")
	}
	if !strings.HasPrefix(a, "ast_") {
		t.Errorf("missing prefix: %s", a)
	}
	if err := ValidateID(a); err != nil {
		t.Errorf("generated ID invalid: %v", err)
	}
	for _, bad := range []string{"", "ast", "ast_zz", "x_0123456789abcdef0123456789abcdef"} {
		if err := ValidateID(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

func TestManifestEntryReductionPercent(t *testing.T) {
	t.Parallel()
	e := ManifestEntry{OriginalSize: 1000, OptimizedSize: 275}
	if got := e.ReductionPercent(); got < 72.4 || got > 72.5 {
		t.Errorf("ReductionPercent() = %v, want ~72.45", got)
	}
	if got := (ManifestEntry{}).ReductionPercent(); got != 0 {
		t.Errorf("ReductionPercent with no original = %v, want 0", got)
	}
}

func TestOptimizationResultImproved(t *testing.T) {
	t.Parallel()
	good := &OptimizationResult{ReductionPercent: 72.5, SSIM: 0.97}
	if !good.Improved() {
		t.Error("72.5% reduction should be an improvement")
	}
	tiny := &OptimizationResult{ReductionPercent: 1}
	if tiny.Improved() {
		t.Error("1% reduction is below MinMeaningfulReductionPercent and should not qualify")
	}
	rejected := &OptimizationResult{ReductionPercent: 80, Rejected: true, Reason: "ssim too low"}
	if rejected.Improved() {
		t.Error("a rejected result must never count as improved")
	}
	var nilResult *OptimizationResult
	if nilResult.Improved() {
		t.Error("nil result must not be an improvement")
	}
}

func TestValidationErrorsAddNestedValidationError(t *testing.T) {
	t.Parallel()
	// Wrapped configuration errors must keep their field name so `vyom doctor`
	// can point at the exact key.
	var errs ValidationErrors
	errs.Add(&ValidationError{Field: "policy.images.quality", Detail: "must be 1-100"})
	joined := errs.Err()
	var ve *ValidationError
	if !errors.As(joined, &ve) {
		t.Fatal("expected a ValidationError")
	}
	if ve.Field != "policy.images.quality" {
		t.Errorf("field = %q", ve.Field)
	}
	if IsValidationError(nil) {
		t.Error("nil is not a validation error")
	}
	if IsValidationError(errors.New("boom")) {
		t.Error("plain error is not a validation error")
	}
}

func TestValidationErrorMessage(t *testing.T) {
	t.Parallel()
	e := &ValidationError{Field: "a.b", Detail: "bad"}
	if got := e.Error(); got != "a.b: bad" {
		t.Errorf("Error() = %q", got)
	}
	withValue := &ValidationError{Field: "a", Detail: "bad", Value: "x"}
	if !strings.Contains(withValue.Error(), `"x"`) {
		t.Errorf("Error() = %q, expected the value to be quoted", withValue.Error())
	}
	if got := (&ValidationError{Detail: "only detail"}).Error(); got != "only detail" {
		t.Errorf("Error() = %q", got)
	}
}

func TestValidationErrorsErrorJoinsAll(t *testing.T) {
	t.Parallel()
	var errs ValidationErrors
	errs.Addf("x", "one")
	errs.Addf("y", "two")
	if got := errs.Error(); !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Errorf("Error() = %q", got)
	}
	if !strings.Contains((&ValidationErrors{}).Error(), "no validation errors") {
		t.Error("empty aggregate should describe itself")
	}
}

func TestPricingConfigConfigured(t *testing.T) {
	t.Parallel()
	if (PricingConfig{}).Configured() {
		t.Error("zero pricing must be unconfigured")
	}
	for _, p := range []PricingConfig{
		{UnitPricePerGB: 0.04}, {StoragePerGBMonth: 0.02}, {PerRequest: 0.0001},
	} {
		if !p.Configured() {
			t.Errorf("%+v should be configured", p)
		}
	}
}

func TestSecretRefs(t *testing.T) {
	t.Parallel()
	if !(SecretRefs{}).IsZero() {
		t.Error("empty refs must report IsZero")
	}
	refs := SecretRefs{ClientSecretEnv: "VYOM_AKAMAI_CLIENT_SECRET", AccessTokenEnv: ""}
	if refs.IsZero() {
		t.Error("populated refs must not report IsZero")
	}
	if got := refs.Names(); len(got) != 1 || got[0] != "VYOM_AKAMAI_CLIENT_SECRET" {
		t.Errorf("Names() = %v", got)
	}
}
