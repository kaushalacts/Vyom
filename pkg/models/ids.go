package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Identifier prefixes. Vyom uses prefixed, sortable-by-nothing random
// identifiers: they are opaque, collision-free and carry no information about
// the object they name (unlike sequential integers, which leak volume).
const (
	PrefixAsset              = "ast"
	PrefixImpact             = "imp"
	PrefixScanJob            = "scn"
	PrefixOptimizationJob    = "opt"
	PrefixPolicy             = "pol"
	PrefixProvider           = "prv"
	PrefixReport             = "rpt"
	PrefixTrafficRecord      = "trf"
	PrefixObject             = "obj"
	PrefixOptimizationResult = "ofr"
)

// NewID returns a new opaque identifier such as "ast_9f2c...".
//
// The first 8 hex characters embed the current time so that IDs created in
// sequence sort roughly by creation time, which makes debugging far easier
// without exposing counts.
func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is unrecoverable; panic loudly rather than
		// silently emitting predictable IDs.
		panic(fmt.Sprintf("models: crypto/rand unavailable: %v", err))
	}
	ms := uint64(time.Now().UTC().UnixMilli())
	for i := 0; i < 8; i++ {
		b[i] = byte(ms >> (8 * uint64(7-i)))
	}
	if prefix == "" {
		return hex.EncodeToString(b[:])
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// idPrefixes is the set of prefixes NewID may emit. ValidateID uses it to
// reject well-formed but unknown identifiers, so that a typo'd ID from an API
// response fails fast instead of silently returning zero rows.
var idPrefixes = map[string]bool{
	PrefixAsset: true, PrefixImpact: true, PrefixScanJob: true,
	PrefixOptimizationJob: true, PrefixPolicy: true, PrefixProvider: true,
	PrefixReport: true, PrefixTrafficRecord: true, PrefixObject: true,
	PrefixOptimizationResult: true,
}

// ValidateID checks the shape of an identifier produced by NewID.
func ValidateID(id string) error {
	prefix, rest, ok := strings.Cut(id, "_")
	if !ok || rest == "" || !idPrefixes[prefix] {
		return NewValidationError("id", "must be <known-prefix>_<32 hex chars>", id)
	}
	if len(rest) != 32 {
		return NewValidationError("id", "must contain 32 hex characters", id)
	}
	if _, err := hex.DecodeString(rest); err != nil {
		return NewValidationError("id", "suffix must be hexadecimal", id)
	}
	return nil
}

// UTC normalizes a timestamp to UTC, Vyom's single time zone on disk and in
// the API. Mixed time zones in reports are a classic source of "why is this
// window wrong" bugs.
func UTC(t time.Time) time.Time { return t.UTC() }

// UTCOrZero normalizes a timestamp, preserving the zero value.
func UTCOrZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}
