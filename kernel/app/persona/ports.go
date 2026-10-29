// SPDX-License-Identifier: MIT

package persona

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The persona family's ports, filled from the kernel handle.
//
// The APPLICATION owns the identity half of them. The persona family is the
// owner's chat defaults: the daemon's default identity — the model the runs
// fall back to and the system instructions they fall back to — and the saved
// prompt library.
//
// The host fills the rest — the config store and the prompt library file's
// bytes — because those are the HOST's files and paths. What the kernel owns
// — the live default identity — the application fills for itself.
//
// The kernel may be absent, and the ports answer for that case: a host with no
// kernel holds no identity, and the answers are the empty ones.

// Identity is the daemon's default identity.
//
// It holds the kernel's own surfaces as CAPABILITIES — the default model and
// the default identity prompt — so a family that asks for the daemon's
// identity never names the kernel's verb to get it.
//
// The names are the persona family's own: a port may not carry the name of the
// kernel verb it stands for, or the adapter that asks the port would be seen
// reaching for the kernel's own verb.
type Identity struct {
	defaultModel func() string
	prompt       func() string
	setPrompt    func(string)
}

// PortsFromKernel fills the identity half of the persona family's ports from
// the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no identity, and
// the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	return Ports{Identity: IdentityFromKernel(k)}
}

// IdentityFromKernel fills the daemon's default identity from the kernel
// handle.
//
// The handle may be nil: a host that holds no kernel holds no identity, and
// the identity answers the empty ones.
func IdentityFromKernel(k *runtime.Kernel) Identity {
	if k == nil {
		return Identity{
			defaultModel: func() string { return "" },
			prompt:       func() string { return "" },
			setPrompt:    func(string) {},
		}
	}
	return Identity{
		defaultModel: func() string { return k.Model() },
		prompt:       func() string { return k.System() },
		setPrompt:    func(prompt string) { k.SetSystem(prompt) },
	}
}

// DefaultModel is the model the runs fall back to.
//
// It is empty when the daemon uses the provider's defaults rather than an
// override.
func (i Identity) DefaultModel() string {
	if i.defaultModel == nil {
		return ""
	}
	return i.defaultModel()
}

// Prompt is the system instructions the runs fall back to.
//
// It is empty when the daemon has no default identity prompt.
func (i Identity) Prompt() string {
	if i.prompt == nil {
		return ""
	}
	return i.prompt()
}

// PromptSet reports whether the daemon has a default identity prompt.
//
// The config surface reports PRESENCE and not the content: the prompt can
// carry proprietary instructions, and the dedicated identity surface returns
// the content for the owner to edit.
func (i Identity) PromptSet() bool {
	return i.Prompt() != ""
}

// SetPrompt replaces the default identity prompt, live — the next default run
// picks it up, with no restart.
//
// Persistence is the host's business: it writes the prompt to the config
// store alongside this.
func (i Identity) SetPrompt(prompt string) {
	if i.setPrompt == nil {
		return
	}
	i.setPrompt(prompt)
}
