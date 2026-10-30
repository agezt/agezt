// SPDX-License-Identifier: MIT

package runtime_test

// The test host helper for the SCRIPT-TOOL FORGE (W3.72).
//
// The forge's FILE is the daemon's — the kernel holds the contract's port and
// never opens a catalogue of its own — so a test that expects a forge opens
// one and injects it, the same way the daemon does.
//
// The forge's LIFECYCLE is the extensions module's (W3.72), so a test that
// expects a working forge also hands the kernel the module's BUILDER — the
// same handover the daemon makes.

import (
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
)

// forgeServiceBuilder is the extensions module's builder, handed to the
// kernel the way the daemon hands it over (W3.72): the module keeps the
// forge's lifecycle, the daemon keeps the names.
func forgeServiceBuilder() extapi.ForgeBuilder {
	return func(deps extapi.ForgeDependencies) extapi.ForgeService {
		return toolforge.NewService(deps)
	}
}
