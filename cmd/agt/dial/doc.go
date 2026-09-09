// SPDX-License-Identifier: MIT

// Package dial owns the control-plane connection for the agt CLI.
// Every agt command that talks to the daemon — and there are
// hundreds, across hundreds of files in package main — used to
// share two private helpers: `dial(stderr) *controlplane.Client`
// and `dialBase(base, stderr) *controlplane.Client`. Both lived
// in main.go and were reachable only from package main, which
// meant the day we wanted to extract any one command into its
// own sub-package we hit a wall: a sub-package cannot import
// `package main`, so the new file would lose its connection
// to the daemon.
//
// This package is the canonical home. The two functions here,
// New and NewAtBase, are the same implementation that lived in
// main.go, lifted verbatim and given tests. The two-failure-mode
// "stale socket" hint (M239) and the "no recorded address" hint
// are preserved exactly so every command that previously printed
// them still does.
//
// Main package keeps a 3-line shim:
//
//	func dial(stderr io.Writer) *controlplane.Client { return dial.New(stderr) }
//
// so the existing 231 call sites in main need no change. New
// sub-packages use this package directly. The shim is the next
// thing to delete once the 231 sites are mechanically rewritten
// (the obvious mechanical-rewrite path is the subject of the
// follow-up sprint).
package dial
