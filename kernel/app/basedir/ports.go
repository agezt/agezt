// SPDX-License-Identifier: MIT

package basedir

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The daemon's data root, filled from the kernel handle.
//
// The APPLICATION owns this, and the family is small on purpose: the data root
// is not any one family's business — it is the place the daemon keeps
// everything, and five families ask it where their own files live.
//
// The journal keeps its segments under it, the sandbox keeps its projects
// under it, the storage keeps its artifacts under it, the policy overlay keeps
// its snapshot under it, the roster keeps its workspaces under it, and the
// config reader reports it to the operator.
//
// The root may be absent — a host that was given no kernel holds none — and
// the port answers for that case with the empty root, which is what a host
// without a daemon has.

// Ports are the daemon's data root.
type Ports struct {
	dir string
}

// PortsFromKernel fills the daemon's data root from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{dir: k.BaseDir()}
}

// Dir is the daemon's data root.
//
// The name is not "BaseDir": a port may not carry the name of the kernel verb
// it stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
func (p Ports) Dir() string { return p.dir }
