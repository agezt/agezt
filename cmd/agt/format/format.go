// SPDX-License-Identifier: MIT

package format

import (
	"fmt"
	"time"
)

// DiskWarnPct is the journal-disk fill percentage at which `agt doctor`
// escalates from "ok" to "warn" (with a hint pointing at `agt backup`).
// 10% is intentionally generous — the journal only grows, so by the
// time it eats a tenth of a disk the operator has already lost most of
// their runway.
const DiskWarnPct = 10.0

// Bytes renders a byte count as B/KB/MB/GB/TB with one decimal (M131).
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Percent returns an integer-percent string ("47") for spent / cap.
// "—" when cap is zero so the caller doesn't need a separate
// branch in its format string.
func Percent(spent, cap int64) string {
	if cap <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d", spent*100/cap)
}

// Time returns the timestamp string verbatim, but renders the
// "never" / "unset" placeholder (empty string or RFC3339 zero
// prefix) as the literal "never" so the column lines up.
func Time(s string) string {
	if s == "" || len(s) >= 5 && s[:5] == "0001-" {
		return "never"
	}
	return s
}

// Duration renders milliseconds as a human-readable duration.
// 0 → "—"; <1s → "Nms"; <60s → "N.Ns"; otherwise "MmNs".
// Distinct from Uptime (which uses seconds + always emits at
// least seconds-level granularity) — runs typically last under
// a minute so sub-second precision matters.
func Duration(ms int64) string {
	switch {
	case ms <= 0:
		return "—"
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		m := ms / 60_000
		s := (ms % 60_000) / 1000
		return fmt.Sprintf("%dm%ds", m, s)
	}
}

// Uptime renders seconds as Hh Mm Ss with leading zero units
// suppressed: "5s", "2m 5s", "1h 2m 5s". Operators eyeball this
// at a glance — a raw integer "3725" is less useful than "1h 2m 5s".
func Uptime(secs int64) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h == 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}

// Number coerces a JSON-decoded scalar to int. Returns 0 for any
// shape it doesn't recognise — callers render this as "0" in their
// table column. JSON numbers come back as float64; explicit ints
// (rare in our wire shapes) are accepted for symmetry.
func Number(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// Str is Number's string twin. It is what we use when a wire
// field is documented as string but a defensive nil-cast would
// otherwise return the empty string anyway — keeping the call
// sites symmetrical with Number.
func Str(v any) string {
	s, _ := v.(string)
	return s
}

// ParseDuration parses a duration string like "1h", "30m", "3600s".
// It first tries time.ParseDuration; on failure it falls back to
// a bare integer (interpreted as seconds). Returns 0 for any
// unparseable input — callers that need to surface "invalid"
// should pre-validate.
func ParseDuration(s string) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	var seconds int64
	fmt.Sscanf(s, "%d", &seconds)
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds) * time.Second
}
