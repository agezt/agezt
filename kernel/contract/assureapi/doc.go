// SPDX-License-Identifier: MIT

// Package assureapi is the contract layer's home for the "do-it-for-sure"
// rows: the completion judgement for one attempt, the record of one run+verify
// cycle, and the outcome of an assured run.
//
// The rows live here because the tool that drives the workboard — and the
// task row it carries — is a PLUGIN, and a plugin may reach the contract and
// the platform layers and nothing else. The workboard's task row carries a
// proof, and a proof carries a verdict: the chain of rows is one chain, and
// it crosses into the contract layer as a whole.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package assureapi
