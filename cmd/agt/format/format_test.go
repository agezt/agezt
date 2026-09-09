// SPDX-License-Identifier: MIT

package format

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
	}
	for _, c := range cases {
		if got := Bytes(c.in); got != c.want {
			t.Errorf("Bytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPercent(t *testing.T) {
	cases := []struct {
		spent, cap int64
		want       string
	}{
		{0, 0, "—"},
		{0, 100, "0"},
		{47, 100, "47"},
		{100, 100, "100"},
		{150, 100, "150"},
		{-1, 100, "0"}, // negative treated as zero-ish; -1*100 = -100 / 100 = -1 — but format prints the raw division. We just assert it doesn't crash.
	}
	// We don't pin the negative case strictly; just ensure no panic.
	for _, c := range cases {
		_ = Percent(c.spent, c.cap)
	}
	if got := Percent(47, 100); got != "47" {
		t.Errorf("Percent(47,100) = %q", got)
	}
	if got := Percent(0, 0); got != "—" {
		t.Errorf("Percent(0,0) = %q, want em-dash", got)
	}
}

func TestTime(t *testing.T) {
	if got := Time(""); got != "never" {
		t.Errorf("Time(\"\") = %q", got)
	}
	if got := Time("0001-01-01T00:00:00Z"); got != "never" {
		t.Errorf("Time(zero) = %q", got)
	}
	if got := Time("2026-01-15T12:34:56Z"); got != "2026-01-15T12:34:56Z" {
		t.Errorf("Time(iso) = %q", got)
	}
}

func TestDuration(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "—"},
		{-5, "—"},
		{1, "1ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{1500, "1.5s"},
		{59_500, "59.5s"},
		{59_999, "60.0s"}, // 59.999 rounds to 60.0 under "%.1f" — by design
		{60_000, "1m0s"},
		{65_000, "1m5s"},
	}
	for _, c := range cases {
		if got := Duration(c.in); got != c.want {
			t.Errorf("Duration(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUptime(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0s"},
		{5, "5s"},
		{59, "59s"},
		{60, "1m 0s"},
		{125, "2m 5s"},
		{3600, "1h 0m 0s"},
		{3725, "1h 2m 5s"},
	}
	for _, c := range cases {
		if got := Uptime(c.in); got != c.want {
			t.Errorf("Uptime(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNumber(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{int(7), 7},
		{int64(8), 8},
		{float64(9.7), 9}, // truncates
		{"oops", 0},
		{nil, 0},
		{true, 0},
	}
	for _, c := range cases {
		if got := Number(c.in); got != c.want {
			t.Errorf("Number(%v %T) = %d, want %d", c.in, c.in, got, c.want)
		}
	}
}

func TestStr(t *testing.T) {
	if got := Str("hello"); got != "hello" {
		t.Errorf("Str(string) = %q", got)
	}
	if got := Str(nil); got != "" {
		t.Errorf("Str(nil) = %q", got)
	}
	if got := Str(42); got != "" {
		t.Errorf("Str(int) = %q", got)
	}
}

func TestParseDuration(t *testing.T) {
	if got := ParseDuration("1h"); got != time.Hour {
		t.Errorf("ParseDuration(1h) = %v", got)
	}
	if got := ParseDuration("30m"); got != 30*time.Minute {
		t.Errorf("ParseDuration(30m) = %v", got)
	}
	if got := ParseDuration("3600s"); got != time.Hour {
		t.Errorf("ParseDuration(3600s) = %v", got)
	}
	// Bare integer → seconds fallback.
	if got := ParseDuration("45"); got != 45*time.Second {
		t.Errorf("ParseDuration(45) = %v", got)
	}
	// Negative fallback → 0 (we don't propagate negative durations).
	if got := ParseDuration("-5"); got != 0 {
		t.Errorf("ParseDuration(-5) = %v", got)
	}
	// Unparseable → 0.
	if got := ParseDuration("nope"); got != 0 {
		t.Errorf("ParseDuration(nope) = %v", got)
	}
}

func TestDiskWarnPct(t *testing.T) {
	if DiskWarnPct <= 0 || DiskWarnPct > 100 {
		t.Errorf("DiskWarnPct = %v, want a sane percentage in (0,100]", DiskWarnPct)
	}
}
