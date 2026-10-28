// SPDX-License-Identifier: MIT

package correlation

import (
	"context"

	"github.com/agezt/agezt/kernel/runtime"
)

// The correlation family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The correlation is the identity one operation
// carries — the journal's rows, the audit's rows and the wake's lineage all
// name it — and the daemon mints a fresh one for every mutating request. The
// application is what knows who asks for one; the host hands over the handle
// and nothing else.
//
// The port holds the kernel's own two surfaces as CAPABILITIES — the mint and
// the actor decoration — so a family that asks for a correlation never names
// the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// correlation to mint, and a context that carries no actor is the context it
// was.

// Ports are the daemon's correlation capabilities.
type Ports struct {
	mint  func() string
	actor func(ctx context.Context, actor, corr string) context.Context
}

// PortsFromKernel fills the correlation family's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		mint:  k.NewCorrelation,
		actor: k.WithActorCorrelation,
	}
}

// Mint mints the correlation of one operation.
//
// The name is not "NewCorrelation": a port may not carry the name of the kernel
// verb it stands for, or the adapter that asks the port would be seen
// reaching for the kernel's own verb.
func (p Ports) Mint() string {
	if p.mint == nil {
		return ""
	}
	return p.mint()
}

// WithActor puts the actor and the correlation into the context, so that
// everything the operation writes names who did it.
//
// A host with no kernel leaves the context as it found it.
func (p Ports) WithActor(ctx context.Context, actor, corr string) context.Context {
	if p.actor == nil {
		return ctx
	}
	return p.actor(ctx, actor, corr)
}
