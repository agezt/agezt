// SPDX-License-Identifier: MIT

// Package okr is AGEZT's durable objectives-and-key-results spine: the layer
// that makes fleet work legible as progress toward goals rather than a flat task
// queue. An Objective owns Key Results; each Key Result links workboard tasks and
// rolls their completion up into a percentage.
//
// The rollup is deliberately fed by DONE tasks, and because the proof gate
// (kernel/proof + kernel/workboard) only lets a criteria-bearing task reach done
// once its acceptance criteria are proven, "done tasks rolling up" is exactly
// "proven tasks rolling up" for gated work — legitimately-completed ungated tasks
// count too. This package stays pure data (no kernel deps): the runtime supplies
// the task-status closure, so Progress is trivially testable.
package okr
