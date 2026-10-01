package models

import "time"

// ScanStatus is the terminal state of a scan job.
//
// A partial scan is a first-class outcome: providers fail, rate limits kick
// in, and a scan that retrieved 97% of a period is far more useful than one
// that reports FAILED and throws the data away.
type ScanStatus string

// Scan statuses.
const (
	ScanStatusPending   ScanStatus = "pending"
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusPartial   ScanStatus = "partial"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusCancelled ScanStatus = "cancelled"
)

// Terminal reports whether no further transitions are possible.
func (s ScanStatus) Terminal() bool {
	switch s {
	case ScanStatusCompleted, ScanStatusPartial, ScanStatusFailed, ScanStatusCancelled:
		return true
	default:
		return false
	}
}

// Valid reports whether s is a known status.
func (s ScanStatus) Valid() bool {
	switch s {
	case ScanStatusPending, ScanStatusRunning, ScanStatusCompleted, ScanStatusPartial, ScanStatusFailed, ScanStatusCancelled:
		return true
	default:
		return false
	}
}

// ScanStage is a step of the scan pipeline, used for progress reporting.
type ScanStage string

// Scan pipeline stages, in execution order.
const (
	StageValidate        ScanStage = "validate"
	StageFetchTraffic    ScanStage = "fetch_traffic"
	StageFetchInventory  ScanStage = "fetch_inventory"
	StageNormalize       ScanStage = "normalize"
	StageCorrelate       ScanStage = "correlate"
	StageDetectMedia     ScanStage = "detect_media"
	StageCalculateImpact ScanStage = "calculate_impact"
	StagePersist         ScanStage = "persist"
	StageReport          ScanStage = "report"
	StageDone            ScanStage = "done"
)

// ScanStages lists the pipeline stages in order.
func ScanStages() []ScanStage {
	return []ScanStage{
		StageValidate, StageFetchTraffic, StageFetchInventory, StageNormalize,
		StageCorrelate, StageDetectMedia, StageCalculateImpact, StagePersist, StageReport,
	}
}

// ScanJob is one execution of the observe/analyze pipeline.
type ScanJob struct {
	ID     string     `json:"id"`
	Status ScanStatus `json:"status"`
	// CurrentStage and ProgressPct drive the progress display.
	CurrentStage ScanStage `json:"current_stage,omitempty"`
	ProgressPct  float64   `json:"progress_pct"`
	// Window is the traffic period the scan covers.
	Window TimeWindow `json:"window"`
	// CDNProvider and StorageProvider name the providers used.
	CDNProvider     string `json:"cdn_provider,omitempty"`
	StorageProvider string `json:"storage_provider,omitempty"`
	// PolicyName is the policy applied during the scan.
	PolicyName string `json:"policy_name,omitempty"`
	// Counts holds per-entity counters for the run.
	Counts ScanCounts `json:"counts"`
	// Warnings are non-fatal problems worth surfacing to the operator.
	Warnings []string `json:"warnings,omitempty"`
	// Error is set when the scan failed; it is redacted before persistence.
	Error string `json:"error,omitempty"`
	// Source distinguishes live providers from fixtures ("akamai", "mock").
	Source     string     `json:"source,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// ScanCounts are the counters a scan maintains as it moves through stages.
type ScanCounts struct {
	// TrafficRecords is the number of CDN URL records retrieved.
	TrafficRecords int64 `json:"traffic_records"`
	// Objects is the number of storage objects listed.
	Objects int64 `json:"objects"`
	// Assets is the number of correlated assets.
	Assets int64 `json:"assets"`
	// MediaAssets is the number of assets whose format Vyom can analyze.
	MediaAssets int64 `json:"media_assets"`
	// MatchedTraffic is the number of traffic records matched to an object.
	MatchedTraffic int64 `json:"matched_traffic"`
	// UnmatchedTraffic is the number of traffic records with no object.
	UnmatchedTraffic int64 `json:"unmatched_traffic"`
	// OrphanObjects is the number of objects never referenced by traffic.
	OrphanObjects int64 `json:"orphan_objects"`
	// UnknownMedia is the number of matched assets of undetectable format.
	UnknownMedia int64 `json:"unknown_media"`
	// Opportunities is the number of assets classified as opportunities.
	Opportunities int64 `json:"opportunities"`
	// Oversized is the number of assets above the policy's warning threshold.
	Oversized int64 `json:"oversized"`
	// BytesDelivered is total bytes delivered in the window.
	BytesDelivered int64 `json:"bytes_delivered"`
	// MediaBytesDelivered is the subset of BytesDelivered from media assets.
	MediaBytesDelivered int64 `json:"media_bytes_delivered"`
	// PotentialSavings is the projected reduction across the corpus.
	PotentialSavings int64 `json:"potential_savings"`
	// Requests is the total request count observed.
	Requests int64 `json:"requests"`
	// Warnings and Errors count recoverable and fatal problems.
	Errors int64 `json:"errors"`
}

// Duration returns the scan duration, or 0 when still running.
func (j *ScanJob) Duration() time.Duration {
	if j.StartedAt.IsZero() {
		return 0
	}
	end := time.Now().UTC()
	if j.FinishedAt != nil {
		end = *j.FinishedAt
	}
	return end.Sub(j.StartedAt)
}

// ReportMetadata describes the report envelope shared by JSON and CSV output.
type ReportMetadata struct {
	ReportID    string     `json:"report_id"`
	ScanID      string     `json:"scan_id,omitempty"`
	ScanJob     ScanJob    `json:"scan"`
	Window      TimeWindow `json:"window"`
	GeneratedAt time.Time  `json:"generated_at"`
	VyomVersion string     `json:"vyom_version"`
	// Warnings carry non-fatal problems, e.g. unmatched traffic counts.
	Warnings []string `json:"warnings,omitempty"`
	// Disclaimers label estimates and causality statements.
	Disclaimers []string `json:"disclaimers,omitempty"`
}

// TrafficSummary aggregates traffic for a report or dashboard tile.
type TrafficSummary struct {
	Window           TimeWindow      `json:"window"`
	Requests         int64           `json:"requests"`
	BytesDelivered   int64           `json:"bytes_delivered"`
	Assets           int64           `json:"assets"`
	MediaBytes       int64           `json:"media_bytes"`
	ImageBytes       int64           `json:"image_bytes"`
	VideoBytes       int64           `json:"video_bytes"`
	OtherBytes       int64           `json:"other_bytes"`
	OversizedAssets  int64           `json:"oversized_assets"`
	Opportunities    int64           `json:"opportunities"`
	PotentialSavings int64           `json:"potential_savings"`
	ByFormat         []FormatTraffic `json:"by_format,omitempty"`
}

// FormatTraffic is the traffic breakdown for one media format.
type FormatTraffic struct {
	Format         string  `json:"format"`
	Kind           string  `json:"kind"`
	Requests       int64   `json:"requests"`
	BytesDelivered int64   `json:"bytes_delivered"`
	Assets         int64   `json:"assets"`
	SharePercent   float64 `json:"share_percent"`
}
