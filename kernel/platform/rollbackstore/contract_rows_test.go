// SPDX-License-Identifier: MIT

package rollbackstore

import "github.com/agezt/agezt/kernel/contract/rollbackapi"

// The platform's checkpoint IS the contract's row.
//
// The console serves the checkpoint row and the platform's store writes it, so
// it is declared once — in the contract (kernel/contract/rollbackapi) — and
// this package's name is an ALIAS of it. An alias cannot drift; a copy can.
//
// The guard is a FUNCTION TYPE on purpose: an assignment would also accept a
// copy with the same fields, but a function type requires identity — so a
// copy declared here would not compile.
var _ func(rollbackapi.Checkpoint) rollbackapi.Checkpoint = func(c rollbackapi.Checkpoint) rollbackapi.Checkpoint {
	return c
}
