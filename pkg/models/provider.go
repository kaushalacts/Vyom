package models

import "time"

// ProviderKind distinguishes the two sides of the correlation.
type ProviderKind string

// Provider kinds.
const (
	// KindCDN serves traffic to end users.
	KindCDN ProviderKind = "cdn"
	// KindStorage holds the origin objects.
	KindStorage ProviderKind = "storage"
)

// ProviderType enumerates supported integrations.
//
// v0.1.0 declares the full set so that configuration, doctor output and the
// docs are stable, but only the types that actually have an implementation
// are marked implemented by internal/provider registry. Declaring a type
// without an implementation is a configuration error, not a silent no-op.
type ProviderType string

// Provider types.
const (
	ProviderAkamai     ProviderType = "akamai"
	ProviderAzureBlob  ProviderType = "azure_blob"
	ProviderCloudFront ProviderType = "cloudfront"
	ProviderCloudflare ProviderType = "cloudflare"
	ProviderFastly     ProviderType = "fastly"
	ProviderS3         ProviderType = "s3"
	ProviderGCS        ProviderType = "gcs"
	ProviderR2         ProviderType = "r2"
	ProviderMinIO      ProviderType = "minio"
	// ProviderMock replays local fixtures. Used by `vyom demo` and by tests;
	// it requires no credentials and no network access.
	ProviderMock ProviderType = "mock"
)

// CDNProviderTypes returns the CDN integrations Vyom knows about.
func CDNProviderTypes() []ProviderType {
	return []ProviderType{ProviderAkamai, ProviderCloudFront, ProviderCloudflare, ProviderFastly}
}

// StorageProviderTypes returns the storage integrations Vyom knows about.
func StorageProviderTypes() []ProviderType {
	return []ProviderType{ProviderAzureBlob, ProviderS3, ProviderGCS, ProviderR2, ProviderMinIO}
}

// Valid reports whether t is a known provider type.
func (t ProviderType) Valid() bool {
	for _, known := range append(CDNProviderTypes(), append(StorageProviderTypes(), ProviderMock)...) {
		if known == t {
			return true
		}
	}
	return false
}

// Kind returns the provider kind a type belongs to.
func (t ProviderType) Kind() ProviderKind {
	switch t {
	case ProviderAkamai, ProviderCloudFront, ProviderCloudflare, ProviderFastly:
		return KindCDN
	case ProviderAzureBlob, ProviderS3, ProviderGCS, ProviderR2, ProviderMinIO:
		return KindStorage
	case ProviderMock:
		return KindStorage // the mock provider can play either role
	default:
		return ""
	}
}

// ProviderConfig is the persisted, non-secret description of a configured
// provider.
//
// Vyom never persists secret values. Options map environment variable names to
// values that are safe to store (hostnames, account names, container names),
// while credentials are referenced by environment variable name only and read
// from the environment at connect time.
type ProviderConfig struct {
	ID   string       `json:"id"`
	Kind ProviderKind `json:"kind"`
	Type ProviderType `json:"type"`
	Name string       `json:"name"`
	// Enabled allows keeping a provider configured but inactive.
	Enabled bool `json:"enabled"`
	// Options holds non-secret settings.
	Options map[string]string `json:"options,omitempty"`
	// Credentials references secret environment variables by name.
	Credentials SecretRefs `json:"credentials,omitempty"`
	// Status and health metadata, maintained by `vyom doctor`.
	Status      ProviderStatus `json:"status"`
	LastChecked time.Time      `json:"last_checked,omitempty"`
	LastError   string         `json:"last_error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// SecretRefs names the environment variables that hold credentials.
//
// Example: {"client_secret_env": "VYOM_AKAMAI_CLIENT_SECRET"}. The referenced
// values are read with os.Getenv at connect time and are never serialized,
// logged or stored.
type SecretRefs struct {
	ClientTokenEnv  string `json:"client_token_env,omitempty"`
	ClientSecretEnv string `json:"client_secret_env,omitempty"`
	AccessTokenEnv  string `json:"access_token_env,omitempty"`
	AccountKeyEnv   string `json:"account_key_env,omitempty"`
	SubscriptionEnv string `json:"subscription_env,omitempty"`
	APIKeyEnv       string `json:"api_key_env,omitempty"`
}

// IsZero reports whether no secret reference is set.
func (s SecretRefs) IsZero() bool {
	return s == SecretRefs{}
}

// Names returns every referenced environment variable name.
func (s SecretRefs) Names() []string {
	all := []string{s.ClientTokenEnv, s.ClientSecretEnv, s.AccessTokenEnv, s.AccountKeyEnv, s.SubscriptionEnv, s.APIKeyEnv}
	out := make([]string, 0, len(all))
	for _, n := range all {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// ProviderStatus is Vyom's view of a provider's configuration health.
type ProviderStatus string

// Provider statuses.
const (
	// ProviderUnconfigured: the provider is selected but has no settings.
	ProviderUnconfigured ProviderStatus = "unconfigured"
	// ProviderConfigured: settings present, credentials present in the environment.
	ProviderConfigured ProviderStatus = "configured"
	// ProviderMissingCredentials: settings present, some credentials absent.
	ProviderMissingCredentials ProviderStatus = "missing_credentials"
	// ProviderUnavailable: an implementation exists but is not built into this binary.
	ProviderUnavailable ProviderStatus = "unavailable"
	// ProviderHealthy: last connectivity check succeeded.
	ProviderHealthy ProviderStatus = "healthy"
	// ProviderError: last connectivity check failed.
	ProviderError ProviderStatus = "error"
)

// OK reports whether the provider can be used for a scan.
func (s ProviderStatus) OK() bool {
	return s == ProviderConfigured || s == ProviderHealthy
}

// ProviderHealth is the result of a connectivity/credential check performed by
// `vyom doctor`. It deliberately reports *configuration* problems; it never
// performs a destructive or billable operation.
type ProviderHealth struct {
	Config ProviderConfig `json:"config"`
	// Status is the resulting provider status.
	Status ProviderStatus `json:"status"`
	// Checks are individual assertions, e.g. "client_secret_env is set".
	Checks []HealthCheck `json:"checks,omitempty"`
	// Message is the headline result.
	Message string `json:"message"`
}

// HealthCheck is one assertion in a doctor run.
type HealthCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	// Hint tells the operator how to fix the problem.
	Hint string `json:"hint,omitempty"`
	// Optional marks a non-blocking check.
	Optional bool `json:"optional,omitempty"`
}

// ID returns a deterministic-ish identifier for the provider config.
func (p ProviderConfig) IDFor() string {
	if p.Name != "" {
		return p.Kind.String() + ":" + p.Name
	}
	return p.Kind.String() + ":" + string(p.Type)
}

// String implements fmt.Stringer.
func (k ProviderKind) String() string { return string(k) }
