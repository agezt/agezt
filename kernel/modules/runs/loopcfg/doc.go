// SPDX-License-Identifier: MIT

// Package loopcfg is the RUNS MODULE's loop-configuration rule: the shape
// every governed run's loop must have, whether it is a root run or a
// delegated child.
//
// It is the seventh slice of the run engine programme (W3.76). W3.76a took
// the durable tickets, W3.76b the steering, W3.76c the run context, W3.76d
// the run registry, W3.76e the prompt, W3.76f the delegation engine; this one
// takes the loop configuration — the last piece of the daemon's run engine
// before the loop itself (kernel/agent) moves.
//
// The family was four pieces in the daemon:
//
//   - loopconfig.go — the assembler: which knobs the loop gets, and how the
//     tool surface, the tool selector, the context budget and the elided-output
//     summarizer are wired;
//   - toolpolicy.go — the tool-set shaping rules: the run's allowlist, the
//     agent profile's allow/deny policy, and the noise policy's prompt-tool
//     pruning;
//   - toolsearch.go — the deferred-discovery rule and the tool that serves it;
//   - the elision half of runexec_helpers.go — the bounded summarizer for
//     dropped tool outputs.
//
// The seam is the same one the delegation engine used (W3.76f): the module
// owns the RULE and the daemon owns the STATE. The rule here is "what a
// governed run's loop looks like"; the state is the daemon's effective
// configuration, its live tool surface (the merged MCP attachments and forged
// script tools), its tool-phase service, its bus, its policy hook, its
// artifact offload port and its model catalog.
//
// The configuration crosses as a PROJECTION, not as a snapshot: the daemon's
// effective configuration is per-run (the operator's live edits plus this
// agent's own overrides), so the daemon reads it for every run and projects
// the knobs the loop needs into loopcfg.Config.
package loopcfg
