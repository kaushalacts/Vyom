package policy

import (
	"fmt"
	"math"
	"strings"
)

// FormatBytes renders a byte count using decimal (SI) units.
//
// Decimal units are deliberate: CDNs bill per GB of 10^9 bytes, so a report
// that said "2.7 GiB" next to an invoice reading "2.8 GB" would send operators
// looking for a discrepancy that does not exist. See docs/architecture.md.
func FormatBytes(b int64) string {
	if b < 0 {
		return "-" + FormatBytes(-b)
	}
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 6; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %s", float64(b)/float64(div), byteUnits[exp])
}

// byteUnits are decimal SI units.
var byteUnits = []string{"kB", "MB", "GB", "TB", "PB", "EB"}

// FormatCount renders an integer count with thin thousands separators.
func FormatCount(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// FormatPercent renders a percentage with one decimal place.
func FormatPercent(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

// FormatDecimal renders a float with the given number of decimal places.
func FormatDecimal(v float64, places int) string {
	return fmt.Sprintf("%.*f", places, v)
}

// PercentDelta renders a relative change between two periods.
//
// A zero or negative baseline is reported as "n/a" rather than as infinity:
// dividing by the previous period's zero traffic is exactly the situation where
// Vyom must not pretend to know a growth rate.
func PercentDelta(previous, current int64) string {
	if previous <= 0 {
		return "n/a (no baseline traffic)"
	}
	delta := (float64(current) - float64(previous)) / float64(previous) * 100
	return fmt.Sprintf("%+.1f%%", delta)
}

// FormatGB converts bytes to decimal gigabytes, the unit providers bill in.
func FormatGB(b int64) float64 { return float64(b) / 1e9 }

// EstimateCost returns the estimated cost of bytes at the given price.
//
// It returns 0 when no price is configured. Callers must not display a cost of
// zero as if it were free: it means "unknown", and they must say so.
func EstimateCost(bytes int64, pricePerGB float64) float64 {
	if pricePerGB <= 0 || bytes <= 0 {
		return 0
	}
	return math.Round(FormatGB(bytes)*pricePerGB*100) / 100
}
