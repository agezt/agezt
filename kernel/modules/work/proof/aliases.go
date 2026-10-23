// SPDX-License-Identifier: MIT

// Package proof is AGEZT's "PROOF > VIBES" record: the durable, checkable
// evidence that a workboard task actually satisfied its acceptance criteria
// before it was allowed to reach done.
//
// The ROWS now live in the contract layer (kernel/contract/proofapi) — W3.31.
// The module keeps their names as aliases, so there is one row and one
// spelling of it: the workboard's task row carries a proof, and a proof
// carries a verdict — the chain crosses into the contract layer as a whole,
// because the tool that drives the workboard is a plugin and a plugin may
// reach the contract and nothing else.
//
// The kernel builds a Proof by running the assure completion judge over a
// task's acceptance criteria and gathering the artifacts plus the hash-chained
// journal range produced under the task's correlation id. A task with no
// acceptance criteria has no Proof and is never gated — you opt into rigor by
// declaring criteria (default-allow posture), you are not restricted by
// default.
package proof

import "github.com/agezt/agezt/kernel/contract/proofapi"

// The rows.
type (
	// Criterion is one acceptance criterion a task must satisfy.
	Criterion = proofapi.Criterion
	// Evidence points at the concrete, verifiable outputs a proof rests on.
	Evidence = proofapi.Evidence
	// Proof is the durable completion record attached to a task.
	Proof = proofapi.Proof
)
