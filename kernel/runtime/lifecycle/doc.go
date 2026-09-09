// SPDX-License-Identifier: MIT

// Package lifecycle is the kernel's run-lifecycle surface extracted into
// a sub-package as the first step of the Day 12 sub-package split.
//
// The package exposes a single Manager type, constructed from the
// kernel's bus + a Suspender callback. The Manager owns the run
// registry's cancel-side concerns: Halt, Resume, CancelRun,
// DrainAndHalt, plus read-side queries (IsHalted, ActiveRuns,
// ActiveRunIDs, NewCorrelation, SubjectForRun).
//
// Why a separate package? The kernel god file was already split into
// four internal files (lifecycle.go, compose.go, accessors.go,
// runexec.go) by Day 11. The next step is to lift the same concerns
// into a real Go sub-package so that:
//
//   - the import graph documents which file owns what (sub-package
//     boundaries > naming conventions in a single package);
//   - lifecycle could in principle be reused by a second kernel
//     variant (e.g. a dry-run replay harness) without dragging in
//     the rest of kernel/runtime;
//   - `go test ./kernel/runtime/lifecycle/...` runs a tight unit
//     suite against the Manager directly, without booting the
//     stores the full Open() needs.
//
// The Manager speaks to the rest of the kernel only through the
// KernelAPI interface (api.go), so the import edge is
// runtime → lifecycle, never the other way. Kernel implements the
// interface implicitly with the public accessors documented on
// Manager.New's parameter.
package lifecycle
