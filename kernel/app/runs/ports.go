// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	appcorrelation "github.com/agezt/agezt/kernel/app/correlation"
	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
	appexecprofile "github.com/agezt/agezt/kernel/app/execprofile"
	apppersona "github.com/agezt/agezt/kernel/app/persona"
	appprovider "github.com/agezt/agezt/kernel/app/provider"
	approster "github.com/agezt/agezt/kernel/app/roster"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/scheduler"
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
//
// The daemon's run verbs are all here: the plain run, the run with a retry
// policy and the cancel. A family that starts a run asks this port, and a
// family that stops one asks it too — the family that owns runs owns the
// run's own end.
type Ports struct {
	launch          func(ctx context.Context, corr, intent string) (string, error)
	launchWithRetry func(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error)
	cancel          func(string) bool
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
		cancel: k.CancelRun,
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

// Cancel stops the run in flight under the correlation.
//
// The run's own end belongs to the family that owns runs: the family that
// starts a run also stops it, and a family that needs a run stopped asks the
// family that owns runs rather than the kernel.
func (p Ports) Cancel(corr string) bool {
	if p.cancel == nil {
		return false
	}
	return p.cancel(corr)
}

// RunPorts binds the run to its host. The kernel is the routed one, so a
// tenant's run stays inside its tenant's journal, bus and tools.
//
// The daemon-side capabilities are filled from the handle by
// RunPortsFromKernel — the roster's lookup, the daemon's default model, the
// correlation, the image admission, the tool registry, the warden's own
// ports, the event bus and the daemon's run verbs. The host-side ones — the
// run fold, the disconnect signal and the remote mirror — are the HOST's
// business, and the host sets them.
//
// Every other family's capability is asked of the family that owns it, so a
// verb is never named twice: the roster answers for the agent lookup, the
// persona family for the daemon's default identity, the correlation family
// for the correlation, the catalog family for the model's own row, the
// provider family for the live provider, the tools family for the registry
// and the runner, and the event bus family for the bus itself.
type RunPorts struct {
	// profile is the roster's own lookup: the agent a run runs as.
	profile func(string) (agentapi.Profile, bool)
	// defaultModel is the daemon's default model — the identity's own.
	defaultModel func() string
	// correlation mints the id a run is journaled under.
	correlation func() string
	// admitImages is the vision gate: what the run may look at.
	admitImages func(context.Context, string, string, string, []string) (runtime.ImageAdmission, error)
	// execProfile is the execution profile family's own ports — the warden.
	execProfile appexecprofile.Ports
	// tools is the tools family's own ports — the registry and the runner.
	tools apptools.Ports
	// provider is the provider family's own ports — the live provider.
	provider appprovider.Ports
	// catalog is the catalog family's own ports — the model's own row.
	catalog appcatalog.Ports
	// identity is the persona family's own ports — the daemon's default prompt.
	identity apppersona.Identity
	// bus is the event bus family's own ports.
	bus appeventbus.Ports
	// launcher is the runs family's own launch port — the daemon's run verbs.
	launcher Ports
	// assured is the daemon's assured loop: run until the answer is assured.
	assured func(context.Context, string, string, int) (string, assureapi.Result, error)
	// subjectForRun is the run's own subject on the bus.
	subjectForRun func(string) string
	// ceiling is the daemon's own run ceiling.
	ceiling func() time.Duration
	// publishRemote publishes a remote-agezt lifecycle event under the run's
	// own subject, so it reaches the run's event stream.
	publishRemote func(string, event.Kind, string, any) error

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
	identity := apppersona.IdentityFromKernel(k)
	bus := appeventbus.PortsFromKernel(k)
	return RunPorts{
		profile:      approster.ProfilePortsFromKernel(k).Get,
		defaultModel: identity.DefaultModel,
		correlation:  appcorrelation.PortsFromKernel(k).Mint,
		admitImages: func(ctx context.Context, corr, model, intent string, images []string) (runtime.ImageAdmission, error) {
			return k.AdmitImages(ctx, corr, model, intent, images)
		},
		execProfile:   appexecprofile.PortsFromKernel(k),
		tools:         apptools.PortsFromKernel(k),
		provider:      appprovider.PortsFromKernel(k),
		catalog:       appcatalog.PortsFromKernel(k),
		identity:      identity,
		bus:           bus,
		launcher:      PortsFromKernel(k),
		assured:       k.RunAssured,
		subjectForRun: k.SubjectForRun,
		ceiling:       k.MaxDuration,
		publishRemote: func(corr string, kind event.Kind, suffix string, payload any) error {
			actor := "agent-" + corr
			_, err := bus.Publish(event.Spec{
				Subject:       "agent." + actor + "." + suffix,
				Kind:          kind,
				Actor:         actor,
				CorrelationID: corr,
				Payload:       payload,
			})
			return err
		},
	}
}

// PlanRunnerPortsFromKernel fills the plan runner's capabilities from the
// daemon's own kernel.
//
// The plan runner's own wiring is the runs family's business: the loop that
// runs a plan's nodes and the daemon's plan verb are named HERE, and the
// adapter never names them. The adapter hands over the daemon, the approvals
// registry and the subscription, and the runner is wired by the family that
// owns it.
//
// A daemon that does not exist yields capabilities that REFUSE — a plan
// cannot run on a daemon that does not exist.
func PlanRunnerPortsFromKernel(k *runtime.Kernel) PlanRunnerPorts {
	if k == nil {
		return PlanRunnerPorts{
			LoopRunner: func() scheduler.LoopRunner { return nil },
			RunPlan: func(context.Context, scheduler.Plan, string) (*scheduler.PlanResult, error) {
				return nil, errors.New("the plan capability is not available")
			},
		}
	}
	return PlanRunnerPorts{LoopRunner: k.LoopRunner, RunPlan: k.RunPlan}
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
	return p.tools.Has(name)
}

// ExecProfile is the execution profile family's own ports — the warden.
func (p RunPorts) ExecProfile() appexecprofile.Ports { return p.execProfile }

// Launcher is the runs family's own launch port — the daemon's run verbs.
func (p RunPorts) Launcher() Ports { return p.launcher }

// Assured runs the intent until the answer is assured.
func (p RunPorts) Assured(ctx context.Context, corr, intent string, maxAttempts int) (string, assureapi.Result, error) {
	if p.assured == nil {
		return "", assureapi.Result{}, errNoRunner
	}
	return p.assured(ctx, corr, intent, maxAttempts)
}

// Subject is the run's own subject on the bus.
func (p RunPorts) Subject(corr string) string {
	if p.subjectForRun == nil {
		return ""
	}
	return p.subjectForRun(corr)
}

// Ceiling is the daemon's own run ceiling.
func (p RunPorts) Ceiling() time.Duration {
	if p.ceiling == nil {
		return 0
	}
	return p.ceiling()
}

// EventBus is the event bus family's own ports.
func (p RunPorts) EventBus() appeventbus.Ports { return p.bus }

// InvokeTool runs a tool on the run's behalf.
//
// The runner is the tools family's own: a run that calls a tool asks the
// family that owns the registry and the runner.
func (p RunPorts) InvokeTool(ctx context.Context, corr, agent, tool string, args json.RawMessage) (toolapi.Result, error) {
	if p.tools.Invoke == nil {
		return toolapi.Result{}, errNoRunner
	}
	return p.tools.Invoke(ctx, corr, agent, tool, args)
}

// Prompt is the daemon's default system prompt — the identity's own.
func (p RunPorts) Prompt() string { return p.identity.Prompt() }

// LiveProvider is the provider a run would talk to.
func (p RunPorts) LiveProvider() llm.Provider { return p.provider.Live() }

// CatalogSnapshot is the catalog the run's model is looked up in.
func (p RunPorts) CatalogSnapshot() *catalogapi.Catalog { return p.catalog.Snapshot() }

// RegisteredTools is the daemon's tool registry.
func (p RunPorts) RegisteredTools() map[string]toolapi.Tool { return p.tools.Registered() }

// PublishRemoteEvent publishes a remote-agezt lifecycle event under the run's
// own subject, so it reaches the run's event stream.
func (p RunPorts) PublishRemoteEvent(corr string, kind event.Kind, suffix string, payload any) error {
	if p.publishRemote == nil {
		return appeventbus.ErrNoBus
	}
	return p.publishRemote(corr, kind, suffix, payload)
}
