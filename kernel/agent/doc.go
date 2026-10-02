// SPDX-License-Identifier: MIT

// Package agent is the canonical first-party single-agent tool-loop
// (DECISIONS B0d) — the orchestration core that the rest of the
// kernel builds on. It runs the two interfaces every adapter in the
// system implements — Provider for LLM adapters and Tool for tool
// adapters, declared in kernel/contract/llm and kernel/contract/toolapi
// and aliased here — in the Run loop that wires them: model request → tool
// dispatch (with Edict policy gate + Warden sandbox) → observation
// → next iteration. The loop is interleaved, not plan-then-execute;
// the DAG scheduler (kernel/scheduler) is a separate layer that
// uses this loop as one of its node types. A panic in any provider
// or tool is contained to the run via deferred recover(); a hostile
// out-of-process plugin cannot crash the daemon.
package agent
