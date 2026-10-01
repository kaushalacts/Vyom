package models

import "testing"

func TestMediaFromContentType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want MediaFormat
	}{
		{"image/jpeg", FormatJPEG},
		{"image/jpg", FormatJPEG},
		{"image/JPEG", FormatJPEG},
		{"image/png; charset=binary", FormatPNG},
		{"image/webp", FormatWebP},
		{"image/avif", MediaFormat("avif")},
		{"text/html", MediaFormat("html")},
		{"", FormatUnknown},
		{"  ", FormatUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := MediaFromContentType(tc.in); got != tc.want {
				t.Errorf("MediaFromContentType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMediaFromPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want MediaFormat
	}{
		{"products/hero-1920.jpg", FormatJPEG},
		{"products/hero-1920.JPG", FormatJPEG},
		{"a/b/c.png?v=4", FormatPNG},
		{"a/b/c.webp#frag", FormatWebP},
		{"noextension", FormatUnknown},
		{"trailingdot.", FormatUnknown},
		{"dir.d/file", FormatUnknown},
		{"", FormatUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := MediaFromPath(tc.in); got != tc.want {
				t.Errorf("MediaFromPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatIsKnownAndKind(t *testing.T) {
	t.Parallel()
	for _, f := range []MediaFormat{FormatJPEG, FormatPNG, FormatWebP} {
		if !f.IsKnown() {
			t.Errorf("%q should be a known format", f)
		}
		if f.Kind() != KindImage {
			t.Errorf("%q kind = %q, want %q", f, f.Kind(), KindImage)
		}
	}
	for _, f := range []MediaFormat{FormatUnknown, "avif", "heic"} {
		if f.IsKnown() {
			t.Errorf("%q must not be reported as supported in v0.1.0", f)
		}
	}
}

func TestKindForFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want MediaKind
	}{
		{"jpeg", KindImage},
		{"png", KindImage},
		{"webp", KindImage},
		{"mp4", KindVideo},
		{"m3u8", KindVideo},
		{"woff2", KindFont},
		{"woff", KindFont},
		{"pdf", KindOther},
		{"", KindUnknown},
	}
	for _, tc := range tests {
		if got := KindForFormat(tc.in); got != tc.want {
			t.Errorf("KindForFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAssetHelpers(t *testing.T) {
	t.Parallel()
	a := Asset{
		Format:         "jpeg",
		Width:          1920,
		Height:         1080,
		SizeBytes:      1_024_000,
		Requests:       1000,
		BytesDelivered: 1_024_000_000,
	}
	if got := a.Pixels(); got != 1920*1080 {
		t.Errorf("Pixels() = %d", got)
	}
	if !a.IsMedia() {
		t.Error("jpeg asset should be media")
	}
	if got := a.AverageSizeBytes(); got != 1_024_000 {
		t.Errorf("AverageSizeBytes() = %d, want 1024000", got)
	}
	noRequests := Asset{Requests: 0}
	if got := noRequests.AverageSizeBytes(); got != 0 {
		t.Errorf("AverageSizeBytes with no requests = %d, want 0 (avoid divide by zero)", got)
	}
	unknown := Asset{Format: "heic", Width: 10}
	if unknown.IsMedia() {
		t.Error("heic must not be media in v0.1.0")
	}
	if got := (Asset{Width: 0, Height: 0}).Pixels(); got != 0 {
		t.Errorf("Pixels() with no dimensions = %d, want 0", got)
	}
}
