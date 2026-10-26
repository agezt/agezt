// SPDX-License-Identifier: MIT

// Package assureapi is the contract layer's home for the "do-it-for-sure"
// rows — the completion judgement for one attempt, the record of one run+verify
// cycle, and the outcome of an assured run — and for the bounded loop that
// produces them.
//
// The rows live here because the tool that drives the workboard — and the
// task row it carries — is a PLUGIN, and a plugin may reach the contract and
// the platform layers and nothing else. The workboard's task row carries a
// proof, and a proof carries a verdict: the chain of rows is one chain, and
// it crosses into the contract layer as a whole.
//
// The LOOP lives here too (W3.64). It runs the task, verifies the answer
// against the ORIGINAL task, and retries with the verifier's gap fed back,
// up to a bounded budget. It was the kernel's last edge into a module, and it
// was never a module: it holds no store, no file, no bus and no clock — only
// two closures and a budget. A pure function of two closures belongs beside
// the rows it produces, so that there is ONE place that knows how a task is
// assured.
//
// The rules of the road:
//
//   - Rows, constants, pure functions and the bounded loop only. No store, no
//     file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package assureapi
