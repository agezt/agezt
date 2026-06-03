// SPDX-License-Identifier: MIT

// Package tenantctx carries the identity of the tenant a run belongs to through the
// run's context.Context, so tools that behave differently per tenant (e.g. the mesh
// remote_run tool selecting a per-tenant peer set) can discover it.
//
// The identity is stamped by the tenant's KERNEL (runtime.Config.TenantID, injected in
// RunWith), NOT by the HTTP layer — because a tenant run can be triggered by a schedule,
// a channel message, or a proactive pulse, none of which carry an HTTP tenant header.
// Stamping at the kernel covers every trigger path uniformly.
//
// The primary (non-multi-tenant) kernel leaves TenantID empty, so Tenant returns "" and
// consumers fall back to their default (global) behaviour — no change for single-tenant.
package tenantctx

import "context"

type ctxKey struct{}

// WithTenant returns a context tagged with the tenant id. An empty id is a no-op (the
// context is returned unchanged), so the primary kernel never tags a run.
func WithTenant(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// Tenant returns the tenant id carried by ctx, or "" when none is set (the primary
// kernel, or any context not derived from a tenant run).
func Tenant(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey{}).(string); ok {
		return id
	}
	return ""
}
