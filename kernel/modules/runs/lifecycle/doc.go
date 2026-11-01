// SPDX-License-Identifier: MIT

// Package lifecycle is the runs module's run-lifecycle surface: Halt,
// Resume, CancelRun, DrainAndHalt, plus the read-side queries
// (IsHalted, ActiveRuns, ActiveRunIDs, NewCorrelation, SubjectForRun).
//
// It moved here from kernel/runtime/lifecycle in W3.76d, together with the
// live-run registry it manages (kernel/modules/runs/registry): the target
// architecture names "run registry" as the runs module's own data, and the
// lifecycle is that data's only writer outside the run engine itself.
//
// The Manager speaks to its host only through the Host interface (api.go),
// and it holds the registry directly — the registry is the module's, so
// there is nothing to reach back for.
package lifecycle
