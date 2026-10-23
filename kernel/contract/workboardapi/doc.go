// SPDX-License-Identifier: MIT

// Package workboardapi is the contract layer's home for the workboard's
// rows: one durable work item (a task), its lifecycle state, the claim,
// dependency, attempt, comment and link records it carries, the retry policy
// and decision, and the creation spec an agent hands over.
//
// The rows live here because the tool that drives the workboard
// (plugins/tools/workboardtool) is a PLUGIN, and a plugin may reach the
// contract and the platform layers and nothing else. A task carries a proof
// and a proof carries a verdict — the chain of rows (task → proof → verdict)
// crosses into the contract layer as a whole.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package workboardapi
