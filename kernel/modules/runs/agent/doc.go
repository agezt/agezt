// SPDX-License-Identifier: MIT

// Package agent is the canonical first-party single-agent tool-loop
// (DECISIONS B0d) — the orchestration core that the rest of the
// kernel builds on. It runs the two interfaces every adapter in the
// system implements — llm.Provider for LLM adapters and toolapi.Tool for
// tool adapters — in the Run loop that wires them: model request → tool
// dispatch (with Edict policy gate + Warden sandbox) → observation
// → next iteration. The loop is interleaved, not plan-then-execute;
// the DAG scheduler (kernel/scheduler) is a separate layer that
// uses this loop as one of its node types. A panic in any provider
// or tool is contained to the run via deferred recover(); a hostile
// out-of-process plugin cannot crash the daemon.
//
// # WHY IT LIVES HERE
//
// The loop is the runs module's engine (W3.76h). It used to live at
// kernel/agent — a top-level package that the archcheck layer map had to
// classify by hand ("module: runs") because the tree did not say so. The
// move makes the tree the statement: the run engine is the runs module's,
// and the special case in the classifier is gone.
//
// The split follows the shape the family already had:
//
//   - the CONTRACT half (llm.Provider, toolapi.Tool, toolapi.ToolDef,
//     toolapi.Result, llm.Message) lives in kernel/contract/llm and
//     kernel/contract/toolapi (W1.1). The loop does NOT re-export it
//     (W3.76i): a caller names llm.Provider or toolapi.Tool directly, so
//     there is one spelling of every row in the tree.
//
//   - the LOOP is here: the Run entry point, the context assembly and
//     compaction, the observation trust rules, the tool gate, the tool memo
//     and the tool selector.
//
//   - the PORTS the loop consumes are defined by the loop (the consumer
//     names the port): the Steerer it consults for directives, the Policy it
//     asks about a tool call. The steering module implements the Steerer;
//     the daemon implements the Policy.
//
// The module never names the daemon: the loop's dependencies are the
// contract rows, the platform (policy, schema, sandbox) and the event bus.
package agent
