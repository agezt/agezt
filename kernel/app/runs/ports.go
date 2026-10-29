// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The runs family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The daemon's own run capability is what two
// adapter surfaces ask for: the roster's operator wake and the workboard's
// dispatch both hand an intent to the daemon and wait for its answer, plainly
// or with a retry policy.
//
// The port holds the kernel's own surfaces as CAPABILITIES, so a family that
// asks for the runner never names the kernel's verbs to get it.
//
// The handle MAY be absent: a host that holds no kernel cannot start a run at
// all, and the port says so instead of calling into nothing.

// errNoRunner is what the port says when the host holds no kernel.
//
// The runner is not a daemon's file that may be missing — the daemon always
// has one — but the HOST that holds no kernel holds none, and it must be told
// the run capability is unavailable rather than handed a call into nothing.
var errNoRunner = errors.New("the run capability is not available")

// Ports are the runs family's capabilities.
type Ports struct {
	launch          func(ctx context.Context, corr, intent string) (string, error)
	launchWithRetry func(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error)
}

// PortsFromKernel fills the runs family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no run capability
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		launch: func(ctx context.Context, corr, intent string) (string, error) {
			return k.RunWith(ctx, corr, intent)
		},
		launchWithRetry: func(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error) {
			return k.RunWithRetry(ctx, corr, intent, pol)
		},
	}
}

// Launch starts one run under the correlation and returns its answer.
//
// A nil policy means a PLAIN run; a policy means the run retries under it.
// The choice between them is the CALLER's — the roster asks for a plain run
// unless it was handed a policy, the workboard asks for a plain run unless
// the policy really retries — and this port only knows how to carry the
// answer back.
//
// The names are not the kernel's: a port that carried the name of the verb it
// stands for would let the adapter name the kernel's verb through the port's
// door.
func (p Ports) Launch(ctx context.Context, corr, intent string, retry *agentapi.RetryPolicy) (string, error) {
	if p.launch == nil || p.launchWithRetry == nil {
		return "", errNoRunner
	}
	if retry != nil {
		return p.launchWithRetry(ctx, corr, intent, *retry)
	}
	return p.launch(ctx, corr, intent)
}
