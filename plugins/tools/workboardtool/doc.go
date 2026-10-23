// SPDX-License-Identifier: MIT

// Package workboardtool is the agent-facing wrapper over AGEZT's durable
// workboard: typed work items that agents can list, create, claim, block,
// complete, comment on, and link to artifacts/workflows/runs.
//
// It is a PLUGIN, so it speaks the contract's rows
// (kernel/contract/workboardapi) and reads the board through the contract's
// port — it never names the module that keeps the store.
package workboardtool
