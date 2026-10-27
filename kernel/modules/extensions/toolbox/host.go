// SPDX-License-Identifier: MIT

package toolbox

// Provenance: SPDX-License-Identifier: MIT Host port: Host + NewHost — this
//             module's implementation of the contract's toolbox port
//             (kernel/contract/toolboxapi). Extracted during W5.4b, when the
//             control plane stopped naming this module and started speaking
//             the contract. Public API added; nothing removed.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/toolboxapi"
)

// Host is this module's implementation of the contract's toolbox port.
//
// The module keeps the work (the catalog, the probes, the package-manager
// call); the contract keeps the rows and the port. The daemon hands the
// control plane a Host, and the control plane never learns which package
// implemented it (W5.4b).
type Host struct{}

// Detect probes every catalog tool and returns the host's snapshot.
func (Host) Detect(ctx context.Context) Inventory { return Detect(ctx) }

// Outdated returns the set of catalog tool names that appear upgradable.
func (Host) Outdated(ctx context.Context) map[string]bool { return Outdated(ctx) }

// Install runs the resolved package-manager command for one named tool.
func (Host) Install(ctx context.Context, name string) InstallResult { return Install(ctx, name) }

// NewHost returns the module's toolbox host. The daemon (the composition root)
// calls it once and hands the port to whoever needs it.
func NewHost() toolboxapi.Host { return Host{} }
