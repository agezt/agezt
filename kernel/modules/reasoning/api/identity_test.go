// SPDX-License-Identifier: MIT

package api

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
)

// The module's rows and the contract's rows must be the SAME types.
//
// The host's methods speak the module's names (kernel/runtime aliases the
// module's api), the tools' Runner interfaces speak the contract's names
// (the council, the conductor and the research tools are plugins, and a plugin
// may only reach the contract layer), and the daemon satisfies both. That
// wiring only compiles when the two are ONE type — so if the module ever
// declares its own copy of a row instead of aliasing the contract's, the
// break shows up at the far end of the tree.
//
// The function-type form is deliberate: an ASSIGNMENT would also accept a copy
// with the same fields, but a function type requires IDENTITY. So these
// assertions say so at the seam, not at the far end.
//
// They are compile-time facts, so the "test" is that this file builds.
var (
	_ func(context.Context, string, string, []CouncilMember, int) (CouncilResult, error) = func(context.Context, string, string, []reasoningapi.CouncilMember, int) (reasoningapi.CouncilResult, error) {
		return reasoningapi.CouncilResult{}, nil
	}
	_ func(context.Context, string, reasoningapi.ConductorConfig) (reasoningapi.ConductorResult, error) = func(context.Context, string, ConductorConfig) (ConductorResult, error) {
		return ConductorResult{}, nil
	}
	_ func(context.Context, string, string, reasoningapi.ResearchOptions) (reasoningapi.ResearchReport, error) = func(context.Context, string, string, ResearchOptions) (ResearchReport, error) {
		return ResearchReport{}, nil
	}
)

// TestTheRowsAreTheContractRows states the same fact at runtime, so a reader
// (and a future change) sees what the assertions above mean: the module's row
// and the contract's row are interchangeable, field for field, and a change to
// one that does not reach the other breaks here.
func TestTheRowsAreTheContractRows(t *testing.T) {
	// The council result round-trips through the contract's shape.
	c := CouncilResult{
		Question:  "q",
		Members:   []CouncilMember{{Seat: "Alpha", Model: "m1"}},
		Rounds:    1,
		Opinions:  []Opinion{{Seat: "Alpha", Model: "m1", Text: "yes"}},
		Consensus: "ship it",
		Dissent:   "hmm",
		AsOf:      "2026-10-23",
		Brief:     "brief",
	}
	var contract reasoningapi.CouncilResult = c
	if contract.Consensus != c.Consensus || contract.Brief != c.Brief {
		t.Fatalf("the council row did not survive the crossing: %+v", contract)
	}

	// The conductor's config and the research's options cross the same way.
	var cfg reasoningapi.ConductorConfig = ConductorConfig{Task: "t", Worker: "w", MaxRounds: 3, Plan: true}
	if cfg.Task != "t" || cfg.MaxRounds != 3 || !cfg.Plan {
		t.Fatalf("the conductor config did not survive the crossing: %+v", cfg)
	}
	var opts reasoningapi.ResearchOptions = ResearchOptions{Model: "m", MaxSources: 5, Verify: true}
	if opts.Model != "m" || opts.MaxSources != 5 || !opts.Verify {
		t.Fatalf("the research options did not survive the crossing: %+v", opts)
	}
}
