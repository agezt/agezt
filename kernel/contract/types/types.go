// SPDX-License-Identifier: MIT

package types

// This file holds the value-type declarations themselves. The package's
// documentation — what belongs here and why — lives in doc.go.

// SubAgentLimits reports the active delegation-governance ceilings
// (M46–M48) for `agt status` (M49). Enabled mirrors whether the
// `delegate` tool is registered; MaxDepth is the EFFECTIVE cap
// (defaulting to 1 when enabled and unset, exactly as runSubAgent
// does); MaxFanout / MaxSpendMicrocents of 0 mean unbounded. Read-only —
// surfaces config the operator set, makes silent governance legible.
//
// The json names are the row's wire: the status operation and the
// introspection tool both publish these ceilings under them, so the row
// carries them once, here.
type SubAgentLimits struct {
	Enabled            bool  `json:"enabled"`
	MaxDepth           int   `json:"max_depth"`
	MaxFanout          int   `json:"max_fanout"`
	MaxSpendMicrocents int64 `json:"max_spend_microcents"`
	MaxTotal           int   `json:"max_total"`
}

// Opinion is one member's position in one round of a council: the seat that
// spoke, the model that spoke for it, the round it belongs to, the text, and
// the error when that seat's call failed.
//
// The runtime records the opinions and the council operation publishes them,
// so the row is declared once, here, and both sides alias it.
type Opinion struct {
	Seat  string `json:"seat"`
	Model string `json:"model"`
	Round int    `json:"round"`
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// TenantInfo is a tenant listing snapshot: the tenant that exists on disk and
// whether its kernel is currently loaded.
//
// The tenant registry owns the row and the tenant operation publishes it, so
// the row is declared once, here, and both sides alias it.
type TenantInfo struct {
	ID      string `json:"id"`
	BaseDir string `json:"base_dir"`
	Open    bool   `json:"open"`
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
