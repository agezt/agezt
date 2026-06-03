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
