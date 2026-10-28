// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The memory family's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the memory needs and
// how each of them answers. The host hands over the handle and nothing else.
//
// The memory is the daemon's: a host that holds none holds a nil manager. The
// kernel's own memory verbs answer for that case — the active records are
// none, and a forget refuses — and the port answers the same way, in one
// place.

// ErrNoMemory is the answer a host with no memory gives.
//
// It is the refusal the kernel already gives its own memory verbs: forgetting
// a record is a decision about a memory that does not exist, and the refusal
// is the answer.
var ErrNoMemory = errors.New("memory: no memory on this daemon")

// Ports are the memory family's capabilities over the daemon's memory.
//
// Both capabilities are held UNEXPORTED: the memory may be absent, and the
// port answers for that case instead of handing the caller a nil object to
// call into.
type Ports struct {
	manager   memoryapi.Manager
	distiller Distiller
}

// PortsFromKernel fills the memory family's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		manager:   k.Memory(),
		distiller: kernelDistiller{k: k},
	}
}

// Manager is the memory manager the memory service takes.
//
// It is nil when the host holds no memory, and the service already knows
// this: it refuses a missing manager instead of panicking.
func (p Ports) Manager() memoryapi.Manager { return p.manager }

// Active lists the memory's active records.
//
// A host with no memory sees no records rather than an error: the cascade
// reads the memory to decide what it may drop, and an empty memory is an
// answer, not a failure.
func (p Ports) Active() ([]memoryapi.Record, error) {
	if p.manager == nil {
		return nil, nil
	}
	return p.manager.Active()
}

// Forget forgets a record.
//
// A host with no memory refuses: forgetting is a decision about a memory that
// does not exist, and the refusal is the answer.
func (p Ports) Forget(id string) (bool, error) {
	if p.manager == nil {
		return false, ErrNoMemory
	}
	return p.manager.Forget("", id)
}

// NewCorrelation mints the correlation a distillation runs under.
//
// The correlation is the host's, not the memory's — but the distillation
// service asks its target for one, and the target is this port.
func (p Ports) NewCorrelation() string {
	if p.distiller == nil {
		return ""
	}
	return p.distiller.NewCorrelation()
}

// DistillBrain consolidates the memory's related records into one.
func (p Ports) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	if p.distiller == nil {
		return memoryapi.BrainDistillReport{}, ErrNoMemory
	}
	return p.distiller.DistillBrain(ctx, corr)
}

// DistillProfile synthesises the operator's profile from the memory.
func (p Ports) DistillProfile(ctx context.Context, corr string) (memoryapi.ProfileReport, error) {
	if p.distiller == nil {
		return memoryapi.ProfileReport{}, ErrNoMemory
	}
	return p.distiller.DistillProfile(ctx, corr)
}

// kernelDistiller is the kernel's distillation, seen through the memory
// family's own narrow interface.
//
// The kernel is the only thing that knows the provider and the model a
// distillation runs under, so the port fills this view from the handle and
// hands the family a CAPABILITY, not the handle it was filled from.
//
// The distillation is reached through the kernel's CHECKED surface — the one
// that refuses a halted kernel — and not through the raw accessor behind it.
// The two are different doors to the same room, and only one of them answers
// when the daemon is going down.
type kernelDistiller struct{ k *runtime.Kernel }

func (d kernelDistiller) NewCorrelation() string { return d.k.NewCorrelation() }

func (d kernelDistiller) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	return d.k.DistillBrain(ctx, corr)
}

func (d kernelDistiller) DistillProfile(ctx context.Context, corr string) (memoryapi.ProfileReport, error) {
	return d.k.DistillProfile(ctx, corr)
}
