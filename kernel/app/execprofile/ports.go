// SPDX-License-Identifier: MIT

package execprofile

import (
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/runtime"
)

// The execution profile family's ports, filled from the kernel handle.
//
// The inventory is the daemon's own picture of where work can run: the tools
// it has registered and the warden that runs them. The port asks for that
// picture WHEN it is asked, so a daemon that gains a tool is seen as it is
// now.
//
// The remote backends — SSH, Kubernetes, Modal, Daytona — come from the
// HOST's environment and not from the kernel: they are not the daemon's
// business.
//
// A host with no daemon holds no tools and no warden, and the port answers
// the empty picture: the profiles that need a tool are unsupported, and the
// ones that do not are still there. That is the truth about a host with no
// daemon, and it is not the same answer as a daemon with no tools.

// PortsFromKernel fills the execution profile's ports from the kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no tools.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{
			Inventory: func() executionprofile.Inventory { return executionprofile.Build(executionprofile.Options{}) },
			Policy:    executionprofile.PolicyFromEnv,
		}
	}
	return Ports{
		Inventory: func() executionprofile.Inventory { return buildInventory(k) },
		Policy:    executionprofile.PolicyFromEnv,
	}
}

// inventory builds the kernel's execution-profile inventory from its tools
// and warden and the host's remote backend configuration, read now.
func buildInventory(k *runtime.Kernel) executionprofile.Inventory {
	return executionprofile.Build(executionprofile.Options{
		Tools:   ToolNames(k.Tools()),
		Warden:  k.Warden(),
		SSH:     executionprofile.SSHConfigFromEnv(),
		K8s:     executionprofile.K8sConfigFromEnv(),
		Modal:   executionprofile.ModalConfigFromEnv(),
		Daytona: executionprofile.DaytonaConfigFromEnv(),
	})
}
