// SPDX-License-Identifier: MIT

// Package haltresume is the home of `agt halt` and `agt resume`.
// The two commands share a body — same flags, same response
// shape — so the package exposes two thin wrappers (Halt,
// Resume) over a private `run(action, …)` rather than two
// parallel copies of the flag-parser. A single command-as-
// package move would have sufficed for the implementation;
// splitting Halt and Resume at the API surface is the smallest
// change to the registration in cmd_register.go.
//
// The package used to be cmd/agt/halt_resume.go in package
// main. It depended on the package-private `dial` shim
// (extracted in Day 5 to cmd/agt/dial) and on the package-
// private `json.MarshalIndent` call (replaced by cmd/agt/jsonout
// in the same Day). With those helpers out of the way, the
// move was mechanical: rename `cmdHaltResume` to the private
// `run`, lift `reasonOrPlaceholder` alongside, expose Halt /
// Resume as the public surface, and update two registration
// lines in cmd_register.go.
//
// The `reasonOrPlaceholder` helper is the kind of UI sugar
// that will eventually move to a `cmd/agt/format` package
// alongside humanBytes / diskWarnPct; for now it stays local
// to its single caller, which is the cheapest possible
// decision.
package haltresume
