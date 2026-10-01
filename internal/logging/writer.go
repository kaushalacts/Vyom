package logging

import (
	"io"
	"os"
	"strings"
)

// stderr returns the process log destination.
//
// Vyom writes logs to stderr, not stdout: stdout carries command output (JSON
// reports, `--json` payloads) and mixing the two would corrupt machine-readable
// output that operators pipe into jq.
func stderr() io.Writer { return os.Stderr }

// redactedQueryParams are URL/query parameters that carry credentials.
//
// Pre-signed CDN and Azure SAS URLs embed an account key, an expiry and a
// signature in the query string. Copying one of those into a log line, an
// error message or a database row is a credential leak, so Vyom rewrites these
// parameters wherever they appear in text.
var redactedQueryParams = []string{
	// Tier 1: actual credentials. Copying any of these into a log line,
	// an error message or a database row is a credential leak.
	"sig", "signature", "token", "access_token", "accesstoken", "refresh_token",
	"policy", "keyid", "awsaccesskeyid",
	"x-amz-signature", "x-amz-credential", "x-amz-security-token",
	"x-goog-signature", "x-goog-credential", "goog-signature",

	// Tier 2: Azure SAS structural parameters. Not secrets on their own, but
	// together they reveal the validity window, permissions and resource a
	// signed URL grants, so Vyom does not emit them either.
	"se", "sp", "sr", "st", "skoid", "sktid", "skt", "ske", "sks", "skv", "sip",

	// Note: "sv" is intentionally absent. It is the SAS API version, a
	// constant that helps operators debug signature failures.
}

// redactQueryParams rewrites credential-bearing parameters in a URL or query
// string. Non-URL text is returned unchanged, so it is safe to call on any log
// message.
func redactQueryParams(s string) string {
	q := strings.IndexByte(s, '?')
	if q < 0 {
		return s
	}
	base, query := s[:q+1], s[q+1:]
	if query == "" {
		return s
	}
	// Only touch the first line: a multi-line log message may contain a URL
	// followed by other text.
	end := strings.IndexAny(query, " \t\n\"'")
	tail := ""
	segment := query
	if end >= 0 {
		segment, tail = query[:end], query[end:]
	}
	parts := strings.Split(segment, "&")
	changed := false
	for i, pair := range parts {
		name, _, hasValue := strings.Cut(pair, "=")
		if !hasValue || !isSecretParam(name) {
			continue
		}
		parts[i] = name + "=" + DefaultPlaceholder
		changed = true
	}
	if !changed {
		return s
	}
	return base + strings.Join(parts, "&") + tail
}

func isSecretParam(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, p := range redactedQueryParams {
		if lower == p {
			return true
		}
	}
	return false
}
