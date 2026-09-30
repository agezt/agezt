// SPDX-License-Identifier: MIT

// Package workflow is the n8n-style workflow engine (M798): durable, named
// graphs of TYPED nodes — trigger, tool, llm, condition, transform, delay —
// wired by edges and carrying data between nodes with {{path}} templates.
// Unlike kernel/planner (intent-in, agent-loop-per-node), a workflow node is
// a precise, deterministic step: THIS tool with THESE args, THIS prompt to
// THIS model, THIS branch on THIS value. The graph is what you see on the
// console canvas; the engine (kernel/runtime) executes it under the same
// governance as everything else — tool nodes pass Edict, llm nodes ride the
// Governor, every step is journaled (workflow.*).
//
// Storage mirrors kernel/standing: one atomic JSON file, journaled CRUD.
package workflow
