// SPDX-License-Identifier: MIT

package skill

import (
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The skill family's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the skill library
// needs and how each of them answers. The host hands over the handle and
// nothing else — it does not decide what the skill library is read from.
//
// The skill library is the DAEMON's (W3.55): a host that was given none holds
// a nil curator. The library's REFUSAL is the answer — not a panic, and not a
// silent "nothing to do" — and the adapter used to spell that refusal itself,
// in three places across two files. The application spells it once, here.
//
// The curator is held UNEXPORTED: the library may be absent, and the port
// answers for that case instead of handing the caller a nil object to call
// into. Every way out of the port is nil-safe.

// Ports are the skill family's capabilities over the daemon's skill library.
type Ports struct {
	curator skillapi.Curator
}

// PortsFromKernel fills the skill family's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{curator: k.Forge()}
}

// List lists the library's skills.
//
// A host with no library sees no skills rather than an error: the cascade
// reads the library to decide what it may drop, and an empty library is an
// answer, not a failure.
func (p Ports) List() ([]skillapi.Skill, error) {
	if p.curator == nil {
		return nil, nil
	}
	return p.curator.List()
}

// Archive archives a skill.
//
// A host with no library refuses, with the very refusal the skill application
// already gives its own services: archiving is a decision about a library that
// does not exist, and the refusal is the answer.
func (p Ports) Archive(id, reason string) error {
	if p.curator == nil {
		return errNoSkillLibrary
	}
	return p.curator.Archive("", id, reason)
}

// Reader is the read-only view the skill service takes.
//
// It is nil when the host holds no library, and the service already knows
// this: it refuses a missing reader instead of panicking.
func (p Ports) Reader() Reader { return p.curator }

// Curation is the curation view the curation service takes.
func (p Ports) Curation() CurationStore { return p.curator }

// Lifecycle is the lifecycle view the lifecycle service takes.
func (p Ports) Lifecycle() LifecycleStore { return p.curator }

// Observation is the observation view the observations service takes.
func (p Ports) Observation() ObservationStore { return p.curator }
