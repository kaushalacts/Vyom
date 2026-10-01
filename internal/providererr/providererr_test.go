package providererr

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestKindFromStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		want   Kind
	}{
		{401, KindAuthentication},
		{403, KindAuthorization},
		{404, KindNotFound},
		{408, KindTimeout},
		{429, KindRateLimited},
		{500, KindUnavailable},
		{503, KindUnavailable},
		{400, KindMalformedResponse},
		{200, KindInternal},
	}
	for _, tc := range tests {
		if got := KindFromStatus(tc.status); got != tc.want {
			t.Errorf("KindFromStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestRetryable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limited", RateLimited("akamai", "FetchTraffic", 5), true},
		{"unavailable", Unavailable("akamai", "FetchTraffic", 503, nil), true},
		{"timeout", Timeout("akamai", "FetchTraffic", nil), true},
		{"authentication", Authentication("akamai", "Connect", nil), false},
		{"authorization", Authorization("akamai", "Connect", nil), false},
		{"configuration", Configuration("akamai", "Connect", "missing host"), false},
		{"not found", NotFound("azure_blob", "GetMetadata", "a.jpg"), false},
		{"unsupported", Unsupported("akamai", "X", "later"), false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Retryable(tc.err); got != tc.want {
				t.Errorf("Retryable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWrapPreservesClassification(t *testing.T) {
	t.Parallel()
	original := Authentication("akamai", "FetchTraffic", errors.New("401 unauthorized"))
	wrapped := Wrap(original, "azure_blob", "ListObjects", KindInternal)

	if !Is(wrapped, KindAuthentication) {
		t.Fatalf("Wrap must preserve an existing classification, got %v", KindOf(wrapped))
	}
	if !strings.Contains(wrapped.Error(), "akamai") {
		t.Errorf("wrapped error lost the original provider: %s", wrapped)
	}
}

func TestWrapClassifiesContextErrors(t *testing.T) {
	t.Parallel()
	if got := KindOf(Wrap(context.Canceled, "p", "op", KindInternal)); got != KindCancelled {
		t.Errorf("KindOf(canceled) = %q, want %q", got, KindCancelled)
	}
	if got := KindOf(Wrap(context.DeadlineExceeded, "p", "op", KindInternal)); got != KindTimeout {
		t.Errorf("KindOf(deadline) = %q, want %q", got, KindTimeout)
	}
	if Wrap(nil, "p", "op", KindInternal) != nil {
		t.Error("Wrap(nil) must return nil")
	}
	if !Is(Wrap(context.DeadlineExceeded, "p", "op", KindInternal), KindTimeout) {
		t.Error("Is must recognize a wrapped deadline")
	}
}

func TestErrorMessageNeverLeaksSecretParams(t *testing.T) {
	t.Parallel()
	r := NewRedactor("super-secret-client-value-1234")
	msg := r.Redact("failed to fetch https://cdn.example.com/a.jpg?sig=abc123&AWSAccessKeyId=AKIA&X-Amz-Signature=deadbeef")

	for _, forbidden := range []string{"abc123", "AKIA", "deadbeef"} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("redacted message still contains %q: %s", forbidden, msg)
		}
	}
	if !strings.Contains(msg, DefaultPlaceholder) {
		t.Errorf("expected placeholder in %q", msg)
	}
	if !strings.Contains(msg, "cdn.example.com/a.jpg") {
		t.Errorf("redaction removed non-secret context: %q", msg)
	}
}

func TestRedactorReplacesLiteralSecrets(t *testing.T) {
	t.Parallel()
	r := NewRedactor("my-real-access-token-value", "short")
	got := r.String("token=my-real-access-token-value other=short")
	if strings.Contains(got, "my-real-access-token-value") {
		t.Errorf("literal secret leaked: %s", got)
	}
	if !strings.Contains(got, "other=short") {
		t.Errorf("redactor removed a value it should not have (too short to be a secret): %s", got)
	}
	if got := (*Redactor)(nil).String("unchanged"); got != "unchanged" {
		t.Errorf("nil redactor must pass text through, got %q", got)
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()
	s := Summarize(RateLimited("akamai", "FetchTraffic", 30))
	if s.Kind != string(KindRateLimited) || s.Provider != "akamai" || s.Op != "FetchTraffic" {
		t.Errorf("unexpected summary: %+v", s)
	}
	if !s.Retryable {
		t.Error("rate limit summary should be retryable")
	}
	if got := Summarize(nil); got.Kind != "" {
		t.Errorf("Summarize(nil) = %+v, want zero value", got)
	}
}

func TestErrorIsTargetsKind(t *testing.T) {
	t.Parallel()
	err := RateLimited("akamai", "FetchTraffic", 1)
	if !errors.Is(err, New(KindRateLimited, "", "", nil)) {
		t.Error("errors.Is should match on kind alone")
	}
	if errors.Is(err, New(KindAuthentication, "", "", nil)) {
		t.Error("errors.Is must not match a different kind")
	}
}

func TestHintCoversEveryKind(t *testing.T) {
	t.Parallel()
	for _, k := range AllKinds() {
		if strings.TrimSpace(Hint(k)) == "" {
			t.Errorf("kind %q has no operator hint", k)
		}
	}
}
