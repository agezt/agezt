// SPDX-License-Identifier: MIT

// Package types is the shared-type home for the value types the
// kernel/runtime sub-packages pass across the host/sub-package
// boundary. Without it, every sub-package that needs SubAgentLimits
// (or PluginInfo, or CouncilMember) would either (a) duplicate the
// type declaration and lose type identity with the host, or (b)
// import kernel/runtime and re-introduce the very circular-import
// problem the Day 12-13 sub-package split solved.
//
// Rules of the road:
//
//   - This package must stay free of kernel/runtime's private
//     fields. A type here is a *value* the kernel passes through its
//     public surface; it never depends on a *Kernel pointer.
//   - It must compile without any kernel/runtime import. Run
//     `go build ./kernel/runtime/types/...` after every change.
//   - New shared types land here, not in a sub-package — that's
//     what "shared" means in this context.
package types

// SubAgentLimits reports the active delegation-governance ceilings
// (M46–M48) for `agt status` (M49). Enabled mirrors whether the
// `delegate` tool is registered; MaxDepth is the EFFECTIVE cap
// (defaulting to 1 when enabled and unset, exactly as runSubAgent
// does); MaxFanout / MaxSpendMicrocents of 0 mean unbounded. Read-only —
// surfaces config the operator set, makes silent governance legible.
type SubAgentLimits struct {
	Enabled            bool
	MaxDepth           int
	MaxFanout          int
	MaxSpendMicrocents int64
	MaxTotal           int
}

// PluginInfo is the daemon-supplied manifest entry for one external
// plugin spawned at startup. Carried on Config so the control plane
// can answer `agt plugin list` without the kernel needing to know
// how plugins are spawned (that's daemon territory).
//
// Fields mirror what's interesting to an operator debugging
// "is my plugin loaded and serving the tools I expected?":
//
//   - Prefix       : namespace tools register under
//   - Path         : binary path the daemon launched
//   - Args         : extra args passed to the binary
//   - ToolCount    : number of tools the plugin exposed
//   - HashPinned   : whether AGEZT_PLUGIN_PINS gated startup
//   - AllowedTools : per-prefix allowlist (nil = no restriction)
type PluginInfo struct {
	Prefix       string
	Path         string
	Args         []string
	ToolCount    int
	HashPinned   bool
	AllowedTools []string
}

// CouncilMember is one seat in the Council of Elders (M839). The
// Governor routes Seat to the right provider; Model is the model id
// on that provider, or empty to use the provider default.
type CouncilMember struct {
	Seat  string `json:"seat"`
	Model string `json:"model"`
}
