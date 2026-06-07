// SPDX-License-Identifier: MIT

package tenantctx

import (
	"context"
	"testing"
)

func TestTenant_DefaultEmpty(t *testing.T) {
	if got := Tenant(context.Background()); got != "" {
		t.Errorf("a context with no tenant should be \"\", got %q", got)
	}
}

func TestWithTenant_RoundTrip(t *testing.T) {
	ctx := WithTenant(context.Background(), "alpha")
	if got := Tenant(ctx); got != "alpha" {
		t.Errorf("Tenant = %q, want alpha", got)
	}
	// Re-tagging replaces the value.
	if got := Tenant(WithTenant(ctx, "beta")); got != "beta" {
		t.Errorf("Tenant after re-tag = %q, want beta", got)
	}
}

func TestWithTenant_EmptyIsNoOp(t *testing.T) {
	// An empty id must not tag the context (the primary kernel passes "").
	base := context.Background()
	ctx := WithTenant(base, "")
	if got := Tenant(ctx); got != "" {
		t.Errorf("empty tenant id should leave the context untagged, got %q", got)
	}
	// "No-op" means the SAME context is returned, not a new one wrapping an empty
	// value. Tenant() returns "" in both cases, so the value check above can't tell
	// them apart — assert identity. Mutation testing (M522) showed the early `return
	// ctx` could be dropped (falling through to WithValue(ctx, key, "")) undetected,
	// which would allocate a wrapper layer on every untenanted run in the primary kernel.
	if ctx != base {
		t.Error("empty tenant id must return the original context unchanged, not a wrapper")
	}
}
