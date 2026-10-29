// SPDX-License-Identifier: MIT

package configtool

import (
	"errors"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The config tool's ports, filled from the kernel handle.
//
// The tool itself is a **plugin**: it may reach the contract layer and the
// platform layer, and it may not name the runtime. The app layer is the
// layer that may — and it answers the contract's port through its **own**
// port, so that the file naming the runtime is the port's file and not the
// adapter's.
//
// The port holds FUNCTIONS: a question is asked when it is asked, and a
// daemon that grows a capability does not grow the port.
//
// A host with no daemon holds no daemon to answer for, and the ports say so
// instead of calling into nothing.

// ErrNoDaemon is the answer a host with no daemon gives.
//
// It is written **once**, here: the port's own refusal, so that the adapter
// and the tool never have to spell "there is no daemon" twice.
var ErrNoDaemon = errors.New("configtool: no daemon on this host")

// Ports are the config tool's capabilities.
//
// Every capability is held UNEXPORTED: the daemon may be absent, and the port
// answers for that case instead of handing the caller a nil object to call
// into.
type Ports struct {
	profile func(string, func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	reload  func() error
}

// PortsFromKernel fills the config tool's ports from the kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no daemon to
// answer for.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		profile: k.UpdateProfile,
		reload: func() error {
			_, _, err := k.Reload()
			return err
		},
	}
}

// Ready reports whether the ports hold a daemon to answer for.
func (p Ports) Ready() bool { return p.profile != nil }

// EditProfile edits the agent named by ref — the agent-scoped write path.
//
// The name is the tool's own: the kernel's verb for it is "UpdateProfile",
// and a port that answers to a kernel verb's name is a verb the adapter may
// name.
func (p Ports) EditProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	if p.profile == nil {
		return agentapi.Profile{}, false, ErrNoDaemon
	}
	return p.profile(ref, mutate)
}

// Rebuild rebuilds the provider in place — the live-apply path.
//
// The catalogue the rebuild produced is dropped: the tool may not name it.
func (p Ports) Rebuild() error {
	if p.reload == nil {
		return ErrNoDaemon
	}
	return p.reload()
}
