// SPDX-License-Identifier: MIT
package plugins

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The plugins family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The plugin manifest is the external-plugin list
// the daemon was given at Open — the rows the operator reads with `agt plugin
// list` — and it is the DAEMON's own file.
//
// The port holds the kernel's own surface as a CAPABILITY — the manifest — so a
// family that asks for the plugins never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// manifest to report.

// Ports are the plugins family's capabilities.
type Ports struct {
	manifest func() []Registration
}

// PortsFromKernel fills the plugins family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no manifest either,
// and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{manifest: k.Plugins}
}

// Manifest is the external-plugin manifest the daemon was given.
//
// The name is not "Plugins": a port may not carry the name of the kernel verb
// it stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
//
// A host with no kernel reports no plugins — the empty manifest is the answer,
// and it is what the daemon says when no external plugin is configured.
func (p Ports) Manifest() []Registration {
	if p.manifest == nil {
		return nil
	}
	return p.manifest()
}
