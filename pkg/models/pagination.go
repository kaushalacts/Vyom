package models

import "time"

// Pagination defaults and bounds. Vyom's API always paginates: a scan can
// legitimately touch millions of rows and an unbounded list endpoint is a
// denial-of-service waiting to happen.
const (
	// DefaultPageLimit is used when a request does not specify a limit.
	DefaultPageLimit = 50
	// MaxPageLimit caps the client-controlled limit.
	MaxPageLimit = 500
)

// SortOrder is the direction of a sort.
type SortOrder string

// Sort orders.
const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// PageRequest describes pagination and sorting for list endpoints.
type PageRequest struct {
	// Limit is the maximum number of items to return.
	Limit int `json:"limit"`
	// Offset is the number of items to skip.
	Offset int `json:"offset"`
	// Sort is the field name, e.g. "bytes_delivered".
	Sort string `json:"sort,omitempty"`
	// Order is the sort direction.
	Order SortOrder `json:"order,omitempty"`
}

// Normalize clamps the request into a safe range and applies defaults.
func (p PageRequest) Normalize() PageRequest {
	out := p
	if out.Limit <= 0 {
		out.Limit = DefaultPageLimit
	}
	if out.Limit > MaxPageLimit {
		out.Limit = MaxPageLimit
	}
	if out.Offset < 0 {
		out.Offset = 0
	}
	if out.Order == "" {
		out.Order = SortDesc
	}
	return out
}

// Validate reports whether the request is acceptable.
func (p PageRequest) Validate(allowedSorts ...string) error {
	var errs ValidationErrors
	if p.Limit < 0 || p.Limit > MaxPageLimit {
		errs.Addf("limit", "must be between 0 and %d", MaxPageLimit)
	}
	if p.Offset < 0 {
		errs.Addf("offset", "must not be negative")
	}
	if p.Order != "" && p.Order != SortAsc && p.Order != SortDesc {
		errs.Addf("order", "must be %q or %q", SortAsc, SortDesc)
	}
	if p.Sort != "" && len(allowedSorts) > 0 {
		ok := false
		for _, s := range allowedSorts {
			if p.Sort == s {
				ok = true
				break
			}
		}
		if !ok {
			errs.Addf("sort", "must be one of %v", allowedSorts)
		}
	}
	return errs.Err()
}

// PageInfo is the pagination envelope returned with every list response.
type PageInfo struct {
	Limit      int   `json:"limit"`
	Offset     int   `json:"offset"`
	Returned   int   `json:"returned"`
	Total      int64 `json:"total"`
	HasMore    bool  `json:"has_more"`
	NextOffset int   `json:"next_offset,omitempty"`
}

// NewPageInfo builds the envelope for a single page of results.
func NewPageInfo(req PageRequest, returned int, total int64) PageInfo {
	req = req.Normalize()
	info := PageInfo{
		Limit:    req.Limit,
		Offset:   req.Offset,
		Returned: returned,
		Total:    total,
	}
	if req.Offset+returned < int(total) {
		info.HasMore = true
		info.NextOffset = req.Offset + returned
	}
	return info
}

// AssetPage is a page of assets.
type AssetPage struct {
	Items []Asset  `json:"items"`
	Page  PageInfo `json:"page"`
}

// ImpactPage is a page of asset impacts.
type ImpactPage struct {
	Items []AssetImpact `json:"items"`
	Page  PageInfo      `json:"page"`
}

// TrafficPage is a page of traffic records.
type TrafficPage struct {
	Items []TrafficRecord `json:"items"`
	Page  PageInfo        `json:"page"`
}

// ScanPage is a page of scan jobs.
type ScanPage struct {
	Items []ScanJob `json:"items"`
	Page  PageInfo  `json:"page"`
}

// OptimizationJobPage is a page of optimization jobs.
type OptimizationJobPage struct {
	Items []OptimizationJob `json:"items"`
	Page  PageInfo          `json:"page"`
}

// PolicyPage is a page of policies.
type PolicyPage struct {
	Items []OptimizationPolicy `json:"items"`
	Page  PageInfo             `json:"page"`
}

// ProviderPage is a page of provider configurations.
type ProviderPage struct {
	Items []ProviderConfig `json:"items"`
	Page  PageInfo         `json:"page"`
}

// ObjectPage is a page of storage objects.
type ObjectPage struct {
	Items []ObjectMetadata `json:"items"`
	Page  PageInfo         `json:"page"`
}

// AssetFilter narrows asset queries. Nil fields are ignored.
type AssetFilter struct {
	// ScanID restricts to assets last seen by one scan.
	ScanID string `json:"scan_id,omitempty"`
	// CDNProvider and StorageProvider restrict by provider.
	CDNProvider     string `json:"cdn_provider,omitempty"`
	StorageProvider string `json:"storage_provider,omitempty"`
	// Format and ContentType restrict by media type.
	Format      string `json:"format,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	// Kind restricts by media category (image, video, ...).
	Kind string `json:"kind,omitempty"`
	// MinSizeBytes and MinBytesDelivered apply bandwidth floors.
	MinSizeBytes      int64 `json:"min_size_bytes,omitempty"`
	MinBytesDelivered int64 `json:"min_bytes_delivered,omitempty"`
	// MinRequests applies a request-count floor.
	MinRequests int64 `json:"min_requests,omitempty"`
	// Search matches URL or storage key.
	Search string `json:"search,omitempty"`
	// Since bounds LastSeen.
	Since time.Time `json:"since,omitempty"`
}

// IsZero reports whether the filter is empty.
func (f AssetFilter) IsZero() bool {
	return f == AssetFilter{}
}
