// SPDX-License-Identifier: MIT

package toolapi

import "context"

// toolAllowlistKey carries the per-run tool allowlist: the set of tools a run
// is allowed to call.
//
// The key lives in the CONTRACT layer because the allowlist is a DECISION
// about a run, not a mechanism of the runtime: the application layer decides
// which tools a run may use, and a host that starts the run states that
// decision without having to name the runtime's module.
type toolAllowlistKey struct{}

// WithTools restricts the run started with this context to the named tools
// only (a per-run allowlist). A non-nil slice — including an EMPTY one (no
// tools at all, for a pure-reasoning / safe one-off run) — activates the
// restriction; passing it is the only way to override, so an unrestricted run
// is simply one where this is never called. Names not registered are ignored.
func WithTools(ctx context.Context, allow []string) context.Context {
	return context.WithValue(ctx, toolAllowlistKey{}, allow)
}

// ToolsFrom returns the per-run tool allowlist and whether one was set.
// ok=false means "no restriction" (use all tools); ok=true with an empty/nil
// slice means "no tools".
func ToolsFrom(ctx context.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	v, ok := ctx.Value(toolAllowlistKey{}).([]string)
	return v, ok
}
