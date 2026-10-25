// SPDX-License-Identifier: MIT

package skill

// Provenance: the explicit-activation directive row. The PARSER moved to the
//             contract layer (kernel/contract/skillapi) during the W3.55
//             slice: the run-time and the run executor both parse the
//             directive, and they may not name this module to reach the
//             parser. The name stays here as an alias, so every existing
//             caller is unchanged.

import "github.com/agezt/agezt/kernel/contract/skillapi"

// ActivationDirective is an explicit skill request embedded at the top of an
// intent. It is intentionally tiny and deterministic: runtime can strip the
// control line from the user-facing task while Forge receives the requested
// skill names/ids for activation.
//
// The row itself lives in the contract layer (kernel/contract/skillapi); the
// name stays here as an alias, so every existing caller is unchanged.
type ActivationDirective = skillapi.ActivationDirective
