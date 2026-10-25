// SPDX-License-Identifier: MIT

package catalog

// Provenance: Catalog operations: NewEmpty. The rules themselves
//             (ProviderList, FindModel, StrictToolArgsNative,
//             ToolCapableAlternative, ToolCapableAlternativeAmong,
//             VisionCapableAmong, BestModelsAcross) moved to the contract
//             layer with the row they are about (W3.51) — the layers above
//             this module may not name it, so the rules had to travel.
//             Code extracted from types.go during the Day-57 god-file split.
//             Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/contract/catalogapi"
)

// NewEmpty returns an empty catalog — no providers, no sources.
//
// The rule lives in the contract layer, with the row; this package keeps the
// name.
func NewEmpty() *Catalog { return catalogapi.NewEmpty() }
