// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"errors"

	appexecprofile "github.com/agezt/agezt/kernel/app/execprofile"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// ErrNoKernel is the answer a host with no daemon gives the run service.
//
// The refusal is written **once**, here, and the resolve path does not spell
// it again.
var ErrNoKernel = errors.New("run kernel unavailable")

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

// RunPorts binds the run to its host. The kernel is the routed one, so a
// tenant's run stays inside its tenant's journal, bus and tools.
//
// The daemon-side capabilities are filled from the handle by
// RunPortsFromKernel — the roster's lookup, the daemon's default model, the
// correlation, the image admission, the tool registry and the warden's own
// ports. The host-side ones — the run fold, the disconnect signal and the
// remote mirror — are the HOST's business, and the host sets them.
//
// The handle itself is still here for the execute path, and it is the LAST
// thing that holds it: a service that reaches for the handle reaches for the
// whole kernel.
type RunPorts struct {
	// profile is the roster's own lookup: the agent a run runs as.
	profile func(string) (agentapi.Profile, bool)
	// defaultModel is the daemon's default model.
	defaultModel func() string
	// correlation mints the id a run is journaled under.
	correlation func() string
	// admitImages is the vision gate: what the run may look at.
	admitImages func(context.Context, string, string, string, []string) (runtime.ImageAdmission, error)
	// hasTool asks whether a tool is registered.
	hasTool func(string) bool
	// execProfile is the execution profile family's own ports — the warden.
	execProfile appexecprofile.Ports
	// kernel is the routed kernel the run executes on. The execute path still
	// reaches for it; the resolve path does not.
	kernel *runtime.Kernel

	// Run folds one of the kernel's journal runs and reports whether the fold
	// knows it. It enriches the terminal answer with the run's own iters,
	// spend and model; an unknown or failing fold only drops those fields.
	Run func(corr string) (Run, bool, error)
	// CancelOnDisconnect cancels the run when the operator's client goes away,
	// so a cancelled run does not keep spending into a closed connection.
	CancelOnDisconnect bool
	// Disconnect reports the client connection closing. It is only consulted
	// when CancelOnDisconnect is set.
	Disconnect func() <-chan struct{}
	// MirrorRemote mirrors a remote-agezt peer's events into this run's
	// stream. The host owns the peer registry, so it provides the mirror.
	MirrorRemote func(ctx context.Context, corr string, meta map[string]string)
}

// RunPortsFromKernel fills the run service's daemon-side ports from the
// kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no daemon to run
// on, and the ports answer for that case.
func RunPortsFromKernel(k *runtime.Kernel) RunPorts {
	if k == nil {
		return RunPorts{}
	}
	tools := apptools.PortsFromKernel(k)
	return RunPorts{
		profile:      k.RosterGet,
		defaultModel: k.Model,
		correlation:  k.NewCorrelation,
		admitImages: func(ctx context.Context, corr, model, intent string, images []string) (runtime.ImageAdmission, error) {
			return k.AdmitImages(ctx, corr, model, intent, images)
		},
		hasTool:     tools.Has,
		execProfile: appexecprofile.PortsFromKernel(k),
		kernel:      k,
	}
}

// Ready reports whether the ports hold a daemon to run on.
func (p RunPorts) Ready() bool { return p.profile != nil }

// Profile is the agent named by ref.
//
// The second answer says whether the daemon holds such an agent.
func (p RunPorts) Profile(ref string) (agentapi.Profile, bool) {
	if p.profile == nil {
		return agentapi.Profile{}, false
	}
	return p.profile(ref)
}

// DefaultModel is the model the daemon's runs fall back to.
func (p RunPorts) DefaultModel() string {
	if p.defaultModel == nil {
		return ""
	}
	return p.defaultModel()
}

// Correlation mints the id a run is journaled under.
func (p RunPorts) Correlation() string {
	if p.correlation == nil {
		return ""
	}
	return p.correlation()
}

// AdmitVision is the vision gate: what the run may look at.
//
// It answers with the request itself when the request carries no images.
func (p RunPorts) AdmitVision(ctx context.Context, corr, model, intent string, images []string) (runtime.ImageAdmission, error) {
	if p.admitImages == nil {
		return runtime.ImageAdmission{Intent: intent, Images: images}, nil
	}
	return p.admitImages(ctx, corr, model, intent, images)
}

// HasTool reports whether a tool is registered.
func (p RunPorts) HasTool(name string) bool {
	if p.hasTool == nil {
		return false
	}
	return p.hasTool(name)
}

// ExecProfile is the execution profile family's own ports — the warden.
func (p RunPorts) ExecProfile() appexecprofile.Ports { return p.execProfile }
