// SPDX-License-Identifier: MIT

package meshctx

import (
	"context"
	"strconv"
	"testing"
)

func TestHop_DefaultZero(t *testing.T) {
	if got := Hop(context.Background()); got != 0 {
		t.Errorf("a context with no hop should be 0, got %d", got)
	}
}

func TestWithHop_RoundTrip(t *testing.T) {
	ctx := WithHop(context.Background(), 3)
	if got := Hop(ctx); got != 3 {
		t.Errorf("Hop = %d, want 3", got)
	}
	// Overwriting replaces the value.
	if got := Hop(WithHop(ctx, 5)); got != 5 {
		t.Errorf("Hop after re-set = %d, want 5", got)
	}
}

func TestWithHop_ClampsNegative(t *testing.T) {
	if got := Hop(WithHop(context.Background(), -2)); got != 0 {
		t.Errorf("negative hop should clamp to 0, got %d", got)
	}
}

func TestMaxHopsFromEnv(t *testing.T) {
	cases := []struct {
		name, env string
		want      int
	}{
		{"unset", "", MaxHops},
		{"valid override", "3", 3},
		{"valid min", "1", 1},
		{"zero falls back", "0", MaxHops},
		{"negative falls back", "-1", MaxHops},
		{"over cap falls back", strconv.Itoa(maxConfigurableHops + 1), MaxHops},
		{"at cap", strconv.Itoa(maxConfigurableHops), maxConfigurableHops},
		{"garbage falls back", "abc", MaxHops},
		{"whitespace trimmed", "  5  ", 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvMaxHops, c.env)
			if got := MaxHopsFromEnv(); got != c.want {
				t.Errorf("MaxHopsFromEnv(%q) = %d, want %d", c.env, got, c.want)
			}
		})
	}
}

// TestMaxHopsConfig_RawAndValidity pins the full MaxHopsConfig contract — the raw value
// and the validOverride flag, not just the effective limit. MaxHopsFromEnv (the only
// caller the other test exercises) discards both, so mutation testing (M521) left the
// validOverride results unpinned: `agt doctor` relies on validOverride=false to flag a
// typo'd override (e.g. "100") that silently fell back to the default — if that flag were
// stuck true, the operator would believe a rejected setting had taken effect.
func TestMaxHopsConfig_RawAndValidity(t *testing.T) {
	cases := []struct {
		name, env string
		wantEff   int
		wantRaw   string
		wantValid bool
	}{
		{"unset", "", MaxHops, "", true},
		{"valid", "3", 3, "3", true},
		{"at cap", strconv.Itoa(maxConfigurableHops), maxConfigurableHops, strconv.Itoa(maxConfigurableHops), true},
		{"over cap falls back, flagged invalid", "100", MaxHops, "100", false},
		{"zero falls back, flagged invalid", "0", MaxHops, "0", false},
		{"garbage falls back, flagged invalid", "abc", MaxHops, "abc", false},
		{"whitespace trimmed in raw", "  5  ", 5, "5", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvMaxHops, c.env)
			eff, raw, valid := MaxHopsConfig()
			if eff != c.wantEff || raw != c.wantRaw || valid != c.wantValid {
				t.Errorf("MaxHopsConfig() = (%d, %q, %v), want (%d, %q, %v)",
					eff, raw, valid, c.wantEff, c.wantRaw, c.wantValid)
			}
		})
	}
}
