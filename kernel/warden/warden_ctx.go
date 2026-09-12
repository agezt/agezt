// SPDX-License-Identifier: MIT

// Warden context: WithCorrelation + CorrelationFrom + WithProfileOverride + ProfileOverrideFrom.
// Code extracted from warden.go during the Day-72 god-file split. Public API unchanged.
package warden


import (
	"context"
)


func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation, or "" if
// none was set.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

// WithProfileOverride returns a child context carrying a per-run requested
// profile. Tools that route through Warden can honor it instead of their
// package default, letting the control plane switch a run between local host and
// warden-backed execution without rebuilding tool instances.
func WithProfileOverride(ctx context.Context, profile Profile) context.Context {
	if profile == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyProfileOverride, profile)
}

// ProfileOverrideFrom extracts the per-run profile set by WithProfileOverride.
func ProfileOverrideFrom(ctx context.Context) (Profile, bool) {
	v, ok := ctx.Value(ctxKeyProfileOverride).(Profile)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
