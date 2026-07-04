// SPDX-License-Identifier: MIT

// Package seat names the execution "seats" a workboard task can be dispatched
// under — the AGEZT answer to Matrix's "choose the right execution seat for the
// job". A seat is a thin, task-facing preset that refines HOW a task runs
// (isolation surface, model tier, tool tier) on top of the agent it is
// dispatched to; the agent still owns identity, soul, and budget.
//
// A seat is pure data: it resolves into the run's existing per-run context
// setters (WithModel/WithModelChain/WithTools and the execution-profile →
// warden override). This package is the seeded catalog of built-in seats; it
// deliberately does not launch work.
package seat

import "strings"

// Seat is a named execution preset. Empty fields mean "inherit from the assigned
// agent / defaults" — a seat only overrides the axes it sets.
type Seat struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// ExecutionProfile selects the isolation surface (warden-family id:
	// local|warden|container). "" = inherit tool defaults.
	ExecutionProfile string `json:"execution_profile,omitempty"`
	// ModelChain, when set, forces this ordered model fallback chain for the run.
	ModelChain []string `json:"model_chain,omitempty"`
	// Tools is the allowlist applied only when RestrictTools is true (an empty
	// allowlist with RestrictTools = a pure-reasoning run).
	Tools         []string `json:"tools,omitempty"`
	RestrictTools bool     `json:"restrict_tools,omitempty"`
	Icon          string   `json:"icon,omitempty"`
	// Builtin marks a seeded, non-deletable seat. Custom operator seats are false.
	Builtin bool `json:"builtin,omitempty"`
}

// ValidExecutionProfile reports whether id is an isolation surface a seat may
// pin: empty (tool defaults) or a warden-family profile. Remote backends
// (ssh/k8s/modal/daytona) are intentionally excluded — they need active backends
// and are only reachable on direct runs.
func ValidExecutionProfile(id string) bool {
	switch strings.TrimSpace(strings.ToLower(id)) {
	case "", "local", "warden", "container":
		return true
	default:
		return false
	}
}

// readOnlyTools is the non-mutating tool set the "reader" seat is confined to.
// Deliberately conservative: search/fetch/read-only browsing and artifact/db
// reads, no shell, file writes, code_exec, http mutations, or memory writes.
var readOnlyTools = []string{"web_search", "fetch", "browser.read", "artifacts", "db"}

// builtins is the seeded catalog, in display order.
var builtins = []Seat{
	{
		ID:          "default",
		Name:        "Default",
		Description: "Inherit everything from the assigned agent — no execution overrides.",
		Icon:        "circle",
	},
	{
		ID:            "reader",
		Name:          "Reader",
		Description:   "Read-only research: search, fetch, and read artifacts/data. No shell, writes, or code execution.",
		Tools:         readOnlyTools,
		RestrictTools: true,
		Icon:          "book-open",
	},
	{
		ID:               "builder",
		Name:             "Builder",
		Description:      "Full tools on the local execution surface — repo edits, tests, shell.",
		ExecutionProfile: "local",
		Icon:             "hammer",
	},
	{
		ID:               "isolated",
		Name:             "Isolated",
		Description:      "Full tools inside the warden sandbox — for untrusted or high-risk work.",
		ExecutionProfile: "warden",
		Icon:             "shield",
	},
}

// Builtins returns the seeded seat catalog (a copy, deterministic order).
func Builtins() []Seat {
	out := make([]Seat, len(builtins))
	for i, s := range builtins {
		c := cloneSeat(s)
		c.Builtin = true
		out[i] = c
	}
	return out
}

// IsBuiltin reports whether id names a seeded seat.
func IsBuiltin(id string) bool {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, s := range builtins {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Get returns the seat with the given id (case-insensitive). The empty id and
// "default" both resolve to the default (inherit-all) seat.
func Get(id string) (Seat, bool) {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		id = "default"
	}
	for _, s := range builtins {
		if s.ID == id {
			c := cloneSeat(s)
			c.Builtin = true
			return c, true
		}
	}
	return Seat{}, false
}

func cloneSeat(s Seat) Seat {
	s.ModelChain = append([]string(nil), s.ModelChain...)
	s.Tools = append([]string(nil), s.Tools...)
	return s
}
