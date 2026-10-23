// SPDX-License-Identifier: MIT

// Package workflowapi is the contract layer's home for the workflow rows: one
// durable, named graph, the typed steps it is made of, the edges that wire
// them, the trigger configuration that starts it, the outcome of a run, and
// the rule that decides whether a graph may run at all.
//
// The rows live here because the tool that authors and runs them is a
// PLUGIN, and a plugin may reach the contract and the platform layers and
// nothing else. The implementation that keeps them — the store, the engine
// that runs the graph, the trigger runner, the templates — stays in the
// module (kernel/modules/workflows/workflow); the module keeps the names its
// own callers use as aliases, so there is one row and one spelling of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package workflowapi
