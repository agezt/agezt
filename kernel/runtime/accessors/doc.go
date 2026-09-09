// SPDX-License-Identifier: MIT

// Package accessors is the kernel's read-mostly surface extracted
// into a sub-package on Day 13 of the runtime split.
//
// The package exposes a single Accessor type, constructed from a
// narrow KernelAPI interface. It is the counterpart to lifecycle
// for the *read* side: getters for every store, the live mutator
// set (Model / System / CouncilMembers / ScheduleEngine), the
// catalog reload, the loop + plan entry points, and the
// Standing/Roster CRUD helpers.
//
// Why a separate package?
//
//   - Like lifecycle, the import graph documents which file owns
//     what. A package boundary beats a naming convention in a 700-
//     line file.
//   - The Accessor can be unit-tested against a fake kernel (the
//     same pattern the lifecycle.Manager tests already use).
//   - Future kernel variants (a dry-run replay harness, a multi-
//     tenant fork) can re-use the Accessor without dragging the
//     store-opening machinery in compose.go along for the ride.
//
// The Accessor speaks to the rest of the kernel only through
// KernelAPI, so the import edge is runtime → accessors, never the
// other way. Kernel implements the interface implicitly with the
// public accessors documented on Accessor.New's parameter.
package accessors
