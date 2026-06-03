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
	ctx := WithTenant(context.Background(), "")
	if got := Tenant(ctx); got != "" {
		t.Errorf("empty tenant id should leave the context untagged, got %q", got)
	}
}
