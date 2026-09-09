// SPDX-License-Identifier: MIT

// Package router is the generic, package-independent core of the
// agt command dispatch loop. It owns the Command shape (name +
// aliases + description + a Run callback) and the helpers every
// dispatcher needs: Suggest for "did you mean …?" on a typo, and
// Execute for the actual lookup-and-dispatch. Package main wires
// the registry by calling Register on every init(); the router
// itself is stateless and has no dependency on the daemon or the
// control plane.
//
// Why this lives in its own package. The previous layout kept the
// Command struct, the registry, and the suggestion logic in
// package main next to 200+ command implementations. Any test had
// to compile all 200 files even if it only cared about Suggest.
// Moving the shape and the helpers to their own package lets
// future domains (cmd/agt/provider, cmd/agt/runs, ...) be tested
// in isolation as they migrate out of package main, and it
// removes a circular risk: a bug in one command's flag parser
// used to take down the test binary for every other command.
package router
