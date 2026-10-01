// Package models contains the shared domain contracts for Vyom.
//
// The types in this package are deliberately free of any provider SDK types so
// that CDN (Akamai, Cloudflare, ...), storage (Azure Blob, S3, ...) and
// optimizer implementations can never leak vendor concepts into the core,
// the API, the persistence layer or the dashboard.
//
// Every type here is part of Vyom's stable API contract: field names and JSON
// tags are additive-only and are documented in docs/api.md.
//
// Terminology used throughout the product:
//
//	Asset               a media object served through the CDN
//	Traffic             requests and bytes delivered for an asset over a period
//	Impact              the relative contribution of an asset to bandwidth
//	Optimization        the process of producing a smaller equivalent asset
//	Optimization Opportunity an asset where optimization is worthwhile
//	Potential Savings   the projected reduction, an upper bound, not a promise
//	Policy              the configurable rule set that defines opportunities
//	Scan                one execution of the observe/analyze pipeline
//	Provider            a CDN or storage integration
package models

import "strings"

// MediaFormat is a normalized, vendor-neutral media container/codec family.
//
// Vyom normalizes provider and HTTP content types into these values so that
// policies, reporting and the dashboard never depend on how a particular CDN
// or storage provider labels a file.
type MediaFormat string

// Media formats supported by Vyom v0.1.0. Additional formats (AVIF, GIF,
// SVG, MP4, ...) are planned; see the roadmap in README.md.
const (
	FormatUnknown MediaFormat = ""
	FormatJPEG    MediaFormat = "jpeg"
	FormatPNG     MediaFormat = "png"
	FormatWebP    MediaFormat = "webp"
)

// MediaKind groups formats into the categories used for traffic breakdown.
type MediaKind string

// Media kinds.
const (
	KindUnknown MediaKind = "unknown"
	KindImage   MediaKind = "image"
	KindVideo   MediaKind = "video"
	KindAudio   MediaKind = "audio"
	KindFont    MediaKind = "font"
	KindOther   MediaKind = "other"
)

// IsKnown reports whether the format is one Vyom can analyze today.
func (f MediaFormat) IsKnown() bool {
	switch f {
	case FormatJPEG, FormatPNG, FormatWebP:
		return true
	default:
		return false
	}
}

// Kind returns the high-level media category of the format.
func (f MediaFormat) Kind() MediaKind {
	switch f {
	case FormatJPEG, FormatPNG, FormatWebP:
		return KindImage
	default:
		return KindUnknown
	}
}

// String implements fmt.Stringer.
func (f MediaFormat) String() string { return string(f) }

// normalizeFormatToken lowercases a format token and folds aliases onto the
// canonical Vyom format names (for example "jpg" and "image/jpeg" -> "jpeg").
func normalizeFormatToken(token string) MediaFormat {
	t := strings.ToLower(strings.TrimSpace(token))
	if t == "" {
		return FormatUnknown
	}
	if i := strings.IndexByte(t, '/'); i >= 0 {
		t = t[i+1:]
	}
	t = strings.TrimPrefix(t, "x-")
	switch t {
	case "jpg", "jpe", "pjpeg":
		return FormatJPEG
	case "png", "apng":
		return FormatPNG
	case "webp":
		return FormatWebP
	default:
		return MediaFormat(t)
	}
}

// MediaFromContentType derives a normalized format from a Content-Type header
// value (for example "image/PNG" -> FormatPNG).
func MediaFromContentType(contentType string) MediaFormat {
	base, _, _ := strings.Cut(contentType, ";")
	return normalizeFormatToken(base)
}

// MediaFromExtension derives a normalized format from a file extension
// (for example ".JPEG" -> FormatJPEG).
func MediaFromExtension(ext string) MediaFormat {
	return normalizeFormatToken(strings.TrimPrefix(ext, "."))
}

// MediaFromPath derives a normalized format from the file extension of a URL
// path or storage key. Query strings and fragments are ignored.
func MediaFromPath(p string) MediaFormat {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	slash := strings.LastIndexByte(p, '/')
	dot := strings.LastIndexByte(p, '.')
	if dot < 0 || (slash >= 0 && dot < slash) || dot == len(p)-1 {
		return FormatUnknown
	}
	return MediaFromExtension(p[dot+1:])
}

// KindForFormat returns the media kind of a format string. It tolerates
// unknown values so that traffic breakdown never loses rows.
func KindForFormat(format string) MediaKind {
	switch MediaFormat(strings.ToLower(strings.TrimSpace(format))) {
	case FormatJPEG, FormatPNG, FormatWebP:
		return KindImage
	case "mp4", "webm", "mov", "m4v", "mkv", "m3u8", "ts", "mpeg", "mpg":
		return KindVideo
	case "mp3", "aac", "ogg", "wav", "flac", "m4a":
		return KindAudio
	case "woff", "woff2", "ttf", "otf", "eot":
		return KindFont
	case "":
		return KindUnknown
	default:
		return KindOther
	}
}
