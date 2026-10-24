// SPDX-License-Identifier: MIT

package reasoningapi

import "context"

// The reasoning surfaces the agent-facing tools drive: the Council of Elders,
// the Conductor and the deep-research harness. Each is a separate panel with
// its own verdict, but all three are the same kind of thing — a reasoning run
// the daemon performs on the agent's behalf — and all three live behind the
// host's reasoning service.
//
// They are PORTS here, not in the tool packages, because the registry hands
// them to the tools (W3.39b). The tool packages keep their own names as
// aliases, so every existing caller — and every fake in every tool's test —
// is unchanged.

// CouncilRunner is the port the council tool drives: convene a multi-model
// panel on a hard question and get a reconciled consensus.
type CouncilRunner interface {
	Council(ctx context.Context, corr, question string, members []CouncilMember, rounds int) (CouncilResult, error)
}

// ConductorRunner is the port the conductor tool drives: an asymmetric,
// verify-driven panel that runs the worker's code and loops until it passes.
type ConductorRunner interface {
	Conduct(ctx context.Context, corr string, cfg ConductorConfig) (ConductorResult, error)
}

// ResearchRunner is the port the research tool drives: decompose a question,
// gather independent sources and synthesize a citation-grounded report.
type ResearchRunner interface {
	Research(ctx context.Context, corr, question string, opts ResearchOptions) (ResearchReport, error)
}

// Runner is the whole reasoning surface — the three ports together.
//
// The registry carries ONE row for it, because the daemon has one reasoning
// service, and a tool that needs only the council still receives it: an
// interface value satisfies a narrower interface when its method set covers
// it, so the three tools keep their own one-verb ports and the daemon keeps
// a single wiring site.
type Runner interface {
	CouncilRunner
	ConductorRunner
	ResearchRunner
}
