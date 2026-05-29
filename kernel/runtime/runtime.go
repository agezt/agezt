// SPDX-License-Identifier: MIT

// Package runtime wires the kernel subsystems (journal + state + bus +
// agent loop + providers + tools) into a single Kernel that the daemon
// hosts and the control plane drives.
//
// One Kernel per Agezt process. Concurrent Run calls are allowed (each
// gets its own correlation_id and ctx); Halt cancels every in-flight run
// and prevents new ones until Resume.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
)

// PluginInfo is the daemon-supplied manifest entry for one
// external plugin spawned at startup. Carried on Config so the
// control plane can answer `agt plugin list` without the kernel
// needing to know how plugins are spawned (that's daemon territory).
//
// Fields mirror what's interesting to an operator debugging
// "is my plugin loaded and serving the tools I expected?":
//   - Prefix       : namespace tools register under
//   - Path         : binary path the daemon launched
//   - Args         : extra args passed to the binary
//   - ToolCount    : number of tools the plugin exposed
//   - HashPinned   : whether AGEZT_PLUGIN_PINS gated startup
//   - AllowedTools : per-prefix allowlist (nil = no restriction)
type PluginInfo struct {
	Prefix       string
	Path         string
	Args         []string
	ToolCount    int
	HashPinned   bool
	AllowedTools []string
}

// Config configures a new Kernel.
type Config struct {
	// BaseDir is the root for journal/, state/, runtime/ subdirs.
	// Defaults to ~/.agezt when constructed via the daemon; tests can
	// inject any directory.
	BaseDir string

	// Provider is the LLM provider the agent loop will drive.
	Provider agent.Provider

	// Tools are the in-process tools advertised to the model.
	Tools map[string]agent.Tool

	// Model is the default model name passed to the provider.
	Model string

	// System is the system prompt prepended to every run.
	System string

	// MaxIter caps tool-call rounds per run (DECISIONS E5).
	MaxIter int

	// Edict is the policy engine that gates each tool call. If nil, a
	// default engine (edict.New(edict.Options{})) is constructed — the
	// runtime is never policy-less.
	Edict *edict.Engine

	// Warden is the process-isolation engine tools use to run external
	// work. If nil, a default cross-platform engine wired to the kernel
	// bus is constructed — the runtime is never warden-less, even when
	// the active profile is ProfileNone.
	Warden warden.Engine

	// Approvals is the HITL queue the policyHook submits to when Edict
	// returns RequiresApproval. If nil, a default in-process registry
	// is constructed. Independent of AskPolicy — the registry is always
	// present so out-of-band callers (agt approve / Telegram / IDE) can
	// list pending requests at any time.
	Approvals *approval.Registry

	// CatalogDir is where catalog/{api,local,custom}.json live. Empty
	// means <BaseDir>/catalog. The kernel loads whatever is on disk on
	// Open (empty catalog if nothing) and installs it into the Governor
	// so pricing reflects the most recent `agt catalog sync`.
	CatalogDir string

	// Catalog, if set, is used instead of loading from CatalogDir.
	// The daemon pre-loads the catalog so it can pick the primary
	// provider; passing it through here avoids a redundant disk read
	// and makes sure runtime and daemon see the same snapshot.
	Catalog *catalog.Catalog

	// Plugins is a manifest of external plugins the daemon spawned
	// at startup. The kernel itself doesn't spawn plugins (that
	// belongs to cmd/agezt's bootstrap), but it carries the
	// manifest so the control plane can surface "what's loaded?"
	// to operators via `agt plugin list`. Nil/empty when no
	// external plugins are configured. Read-only after Open.
	Plugins []PluginInfo

	// OnReload is invoked by Kernel.Reload() AFTER the catalog snapshot
	// has been refreshed from disk. The closure is supplied by the
	// daemon and is expected to:
	//   1. Re-read the credentials vault
	//   2. Re-run the primary-provider selection against the fresh
	//      catalog + lookup
	//   3. Replace the Governor registry's primary entry atomically
	//      (via governor.Registry.Replace).
	//
	// Keeping provider-construction in the daemon (not the runtime)
	// preserves the existing separation: kernel/runtime stays
	// provider-agnostic; cmd/agezt owns the build logic. Nil is
	// allowed — Kernel.Reload then refreshes only the catalog snapshot.
	OnReload func() error
}

// Kernel is the running Agezt instance.
type Kernel struct {
	cfg Config

	journal   *journal.Journal
	state     *state.FileStore
	bus       *bus.Bus
	edict     *edict.Engine
	warden    warden.Engine
	approvals *approval.Registry
	scheduler *scheduler.Executor

	catalogStore *catalog.Store
	catalog      *catalog.Catalog // snapshot — refreshable via ReloadCatalog

	mu     sync.Mutex
	halted bool
	runs   map[string]context.CancelFunc // correlation_id → cancel

	startTime time.Time // wall-clock at Open() — powers `agt status` uptime
}

// ErrHalted is returned by Run when the kernel is in halt state.
var ErrHalted = errors.New("runtime: kernel is halted")

// Open initialises the journal, state, and bus under cfg.BaseDir and
// returns a ready-to-use Kernel.
func Open(cfg Config) (*Kernel, error) {
	if cfg.BaseDir == "" {
		return nil, errors.New("runtime: BaseDir required")
	}
	if cfg.Provider == nil {
		return nil, errors.New("runtime: Provider required")
	}

	j, err := journal.Open(filepath.Join(cfg.BaseDir, "journal"), journal.Options{})
	if err != nil {
		return nil, fmt.Errorf("runtime: journal: %w", err)
	}
	st, err := state.Open(filepath.Join(cfg.BaseDir, "state"))
	if err != nil {
		j.Close()
		return nil, fmt.Errorf("runtime: state: %w", err)
	}
	eng := cfg.Edict
	if eng == nil {
		eng = edict.New(edict.Options{})
	}
	kbus := bus.New(j)
	w := cfg.Warden
	if w == nil {
		w = warden.New(kbus)
	}
	apr := cfg.Approvals
	if apr == nil {
		apr = approval.New(approval.Config{Bus: kbus})
	}
	sched := scheduler.New(scheduler.Config{Bus: kbus})

	catDir := cfg.CatalogDir
	if catDir == "" {
		catDir = filepath.Join(cfg.BaseDir, "catalog")
	}
	catStore := catalog.NewStore(catDir)
	cat := cfg.Catalog
	if cat == nil {
		loaded, err := catStore.Load()
		if err != nil {
			j.Close()
			st.Close()
			return nil, fmt.Errorf("runtime: catalog load: %w", err)
		}
		cat = loaded
	}
	governor.SetCatalog(cat)

	k := &Kernel{
		cfg:          cfg,
		journal:      j,
		state:        st,
		bus:          kbus,
		edict:        eng,
		warden:       w,
		approvals:    apr,
		scheduler:    sched,
		catalogStore: catStore,
		catalog:      cat,
		runs:         make(map[string]context.CancelFunc),
		startTime:    time.Now(),
	}
	return k, nil
}

// Close stops the bus, then closes state and the journal. Pending runs
// are cancelled via Halt before close.
func (k *Kernel) Close() error {
	k.Halt() // cancel any in-flight runs first
	k.bus.Close()
	if err := k.state.Close(); err != nil {
		return err
	}
	return k.journal.Close()
}

// Journal exposes the underlying journal for read-only inspection (used by
// the control plane's `why` and `journal verify`).
func (k *Kernel) Journal() *journal.Journal { return k.journal }

// Bus exposes the underlying bus for the control plane to attach
// subscribers (used by `run` to stream events back to the client).
func (k *Kernel) Bus() *bus.Bus { return k.bus }

// State exposes the underlying state store.
func (k *Kernel) State() *state.FileStore { return k.state }

// Edict exposes the policy engine for read/configure (e.g. `agt trust`
// commands when they land).
func (k *Kernel) Edict() *edict.Engine { return k.edict }

// Warden exposes the isolation engine. Tools that need to run external
// work should accept this rather than calling os/exec directly.
func (k *Kernel) Warden() warden.Engine { return k.warden }

// Approvals exposes the HITL queue so the control plane can list
// pending requests and out-of-band callers (agt approve / Telegram /
// IDE) can submit decisions.
func (k *Kernel) Approvals() *approval.Registry { return k.approvals }

// Scheduler exposes the DAG executor for callers that want to run a
// pre-built Plan via Kernel.RunPlan.
func (k *Kernel) Scheduler() *scheduler.Executor { return k.scheduler }

// Provider exposes the live agent.Provider so callers (notably the
// planner, which needs an LLM round-trip to generate a DAG) can
// reuse the kernel's configured routing without re-wiring catalog
// lookup. Returns the Governor instance when one was passed via
// Config.Provider; a hot reload via Replace updates this pointer's
// underlying chain atomically, so cached callers stay correct.
func (k *Kernel) Provider() agent.Provider { return k.cfg.Provider }

// Tools returns the live in-process tool map exactly as the agent
// loop sees it. Read-only — callers must not mutate the returned
// map. Used by the control plane to power `agt tool list`, which
// is operator visibility into what's actually wired into the
// daemon (vs what `agt catalog list` claims about providers).
func (k *Kernel) Tools() map[string]agent.Tool { return k.cfg.Tools }

// ActiveRuns returns the number of in-flight Run / RunPlan
// invocations. Used by `agt status` to surface "is anything
// happening?" without scraping the bus. Safe under concurrent
// Run starts/completes — takes the same mutex Halt does.
func (k *Kernel) ActiveRuns() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.runs)
}

// StartTime returns the wall-clock time Open() returned. Used by
// `agt status` to compute uptime; not adjusted by Reload or any
// in-process state change, so it reflects "since this process
// started" rather than "since the kernel was last reconfigured".
func (k *Kernel) StartTime() time.Time { return k.startTime }

// Plugins returns the external-plugin manifest the daemon
// supplied at Open(). Read-only — callers must not mutate the
// slice. Used by the control plane to power `agt plugin list`;
// returns nil when no external plugins are configured.
func (k *Kernel) Plugins() []PluginInfo { return k.cfg.Plugins }

// Catalog returns the currently-loaded provider/model catalog. The
// returned pointer is the live snapshot; callers should treat it as
// read-only and re-call after ReloadCatalog if they need fresh data.
func (k *Kernel) Catalog() *catalog.Catalog {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.catalog
}

// CatalogStore returns the on-disk store backing the catalog, so the
// control plane can drive `agt catalog sync` writes.
func (k *Kernel) CatalogStore() *catalog.Store { return k.catalogStore }

// Reload refreshes both the catalog snapshot AND the live provider
// registry. Catalog reload is always performed; provider rebuild runs
// when Config.OnReload is non-nil. This is the operator-facing hot-
// reload entry point invoked by the control plane's `provider.reload`
// command (and, by extension, `agt provider reload`).
//
// Returns (catalog, providersReloaded, err). providersReloaded is
// true when OnReload ran successfully; false when OnReload was nil
// (catalog-only reload) or returned an error.
func (k *Kernel) Reload() (*catalog.Catalog, bool, error) {
	cat, err := k.ReloadCatalog()
	if err != nil {
		return nil, false, err
	}
	if k.cfg.OnReload == nil {
		return cat, false, nil
	}
	if err := k.cfg.OnReload(); err != nil {
		return cat, false, fmt.Errorf("runtime: provider reload: %w", err)
	}
	return cat, true, nil
}

// ReloadCatalog re-reads catalog files from disk and re-installs the
// snapshot into the Governor. Called after `agt catalog sync` and
// after Ollama discovery completes so live pricing reflects the new
// data immediately.
func (k *Kernel) ReloadCatalog() (*catalog.Catalog, error) {
	cat, err := k.catalogStore.Load()
	if err != nil {
		return nil, err
	}
	k.mu.Lock()
	k.catalog = cat
	k.mu.Unlock()
	governor.SetCatalog(cat)
	return cat, nil
}

// LoopRunner returns a closure suitable for scheduler.LoopNode.Runner.
// The closure drives one agent.Run end-to-end via the kernel's
// configured Provider/Tools/Policy hook, using the plan-derived
// correlation ID so events stay linked under `agt why`.
func (k *Kernel) LoopRunner() scheduler.LoopRunner {
	return func(ctx context.Context, intent, corr string) (string, error) {
		return k.RunWith(ctx, corr, intent)
	}
}

// RunPlan executes a pre-built Plan through the kernel's scheduler.
// Honors Halt: refuses to start when halted; in-flight nodes are
// cancelled when Halt is called mid-plan. PlanID is the correlation
// ID for the whole plan; if empty, the scheduler mints one.
func (k *Kernel) RunPlan(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
	k.mu.Lock()
	if k.halted {
		k.mu.Unlock()
		return nil, ErrHalted
	}
	if planID == "" {
		planID = "plan-" + ulid.New()
	}
	runCtx, cancel := context.WithCancel(ctx)
	k.runs[planID] = cancel
	k.mu.Unlock()

	defer func() {
		k.mu.Lock()
		delete(k.runs, planID)
		k.mu.Unlock()
		cancel()
	}()

	return k.scheduler.Run(runCtx, plan, planID)
}

// policyHook adapts the kernel's Edict engine to the agent.Policy
// signature the tool-loop expects. It is called once per ToolCall,
// before invocation.
//
// Three paths:
//
//  1. Hard-deny / unknown-cap / L0 / AskDeny → Allow=false, run skipped.
//  2. L4 Allow or AskAllow folded Ask → Allow=true.
//  3. AskPrompt landed on Ask-class → submit to approval.Registry and
//     block on the operator's decision. Grant flips Allow=true; deny /
//     timeout / cancel keep Allow=false with the verdict reason.
//
// The ctx passed in is the per-run context; cancellation (Halt) flows
// through to Submit and surfaces as DecisionCancel.
func (k *Kernel) policyHook(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
	cap := edict.CapabilityForToolCall(tc.Name, tc.Input)
	out := k.edict.Decide(cap, string(tc.Input))

	verdict := agent.PolicyVerdict{
		Allow:      out.Decision == edict.DecisionAllow,
		Capability: string(out.Capability),
		Reason:     out.Reason,
		WouldAsk:   out.WouldAsk,
		HardDenied: out.HardDenied,
	}

	if !out.RequiresApproval {
		return verdict
	}

	// Live HITL: pause the tool-loop, route the request through the
	// approval queue, block until decided.
	actor := actorFromCtx(ctx)
	corr := correlationFromCtx(ctx)
	res := k.approvals.Submit(ctx, approval.SubmitSpec{
		Capability:    string(out.Capability),
		ToolName:      tc.Name,
		Input:         string(tc.Input),
		Reason:        out.Reason,
		Actor:         actor,
		CorrelationID: corr,
	})
	switch res.Decision {
	case approval.DecisionGrant:
		verdict.Allow = true
		verdict.Reason = "approval granted by " + res.ResolvedBy
	default:
		verdict.Allow = false
		verdict.Reason = fmt.Sprintf("approval %s: %s", res.Decision, res.Reason)
	}
	return verdict
}

// per-run context keys used by RunWith → policyHook to carry the
// actor/correlation IDs into approval.Submit so audit events stay
// linked to the originating task.
type ctxKey int

const (
	ctxKeyActor ctxKey = iota
	ctxKeyCorrelation
)

func actorFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyActor).(string); ok {
		return v
	}
	return ""
}

func correlationFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

// IsHalted reports whether Run will refuse to start.
func (k *Kernel) IsHalted() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.halted
}

// Halt cancels every in-flight run and prevents new ones. It emits a
// `halt` event to the journal so the action is auditable.
func (k *Kernel) Halt() {
	k.mu.Lock()
	if k.halted {
		k.mu.Unlock()
		return
	}
	k.halted = true
	cancels := make([]context.CancelFunc, 0, len(k.runs))
	for _, c := range k.runs {
		cancels = append(cancels, c)
	}
	k.runs = make(map[string]context.CancelFunc)
	k.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "kernel.halt",
		Kind:    event.KindHalt,
		Actor:   "kernel",
		Payload: map[string]int{"cancelled_runs": len(cancels)},
	})
}

// Resume clears the halt flag, allowing new runs. Already-cancelled runs
// stay cancelled; only future Run calls will succeed.
func (k *Kernel) Resume() {
	k.mu.Lock()
	if !k.halted {
		k.mu.Unlock()
		return
	}
	k.halted = false
	k.mu.Unlock()
	_, _ = k.bus.Publish(event.Spec{
		Subject: "kernel.resume",
		Kind:    event.KindResume,
		Actor:   "kernel",
	})
}

// NewCorrelation mints a fresh correlation ID suitable for RunWith. Useful
// for callers (e.g. the control plane) that want to subscribe to the
// per-run event subject *before* starting the run.
func (k *Kernel) NewCorrelation() string { return "run-" + ulid.New() }

// SubjectForRun returns the bus subject pattern that matches every event
// emitted by the agent.Run identified by corr. Use with k.Bus().Subscribe
// to stream a single run's events without seeing others.
func (k *Kernel) SubjectForRun(corr string) string { return "agent.agent-" + corr + ".>" }

// Run executes one tool-loop end-to-end and returns (answer, corr, err).
// It mints a correlation ID internally; for the subscribe-then-run flow
// the control plane uses, see NewCorrelation + RunWith.
func (k *Kernel) Run(ctx context.Context, intent string) (string, string, error) {
	corr := k.NewCorrelation()
	ans, err := k.RunWith(ctx, corr, intent)
	return ans, corr, err
}

// RunWith executes one tool-loop using the supplied correlation ID.
// If the kernel is halted before this Run starts, returns ErrHalted. If
// Halt is called during the Run, ctx is cancelled and RunWith returns
// context.Canceled.
func (k *Kernel) RunWith(ctx context.Context, corr, intent string) (string, error) {
	if corr == "" {
		return "", errors.New("runtime: correlation id required")
	}
	k.mu.Lock()
	if k.halted {
		k.mu.Unlock()
		return "", ErrHalted
	}
	runCtx, cancel := context.WithCancel(ctx)
	k.runs[corr] = cancel
	k.mu.Unlock()

	defer func() {
		k.mu.Lock()
		delete(k.runs, corr)
		k.mu.Unlock()
		cancel()
	}()

	actor := "agent-" + corr
	// Stash actor + correlation on the ctx so the policyHook can
	// thread them into approval.Submit (the agent.Policy contract
	// doesn't expose them directly).
	runCtx = context.WithValue(runCtx, ctxKeyActor, actor)
	runCtx = context.WithValue(runCtx, ctxKeyCorrelation, corr)
	return agent.Run(runCtx, agent.LoopConfig{
		Provider:      k.cfg.Provider,
		Tools:         k.cfg.Tools,
		Bus:           k.bus,
		Model:         k.cfg.Model,
		System:        k.cfg.System,
		MaxIter:       k.cfg.MaxIter,
		Actor:         actor,
		CorrelationID: corr,
		Policy:        k.policyHook,
	}, intent)
}

// Why returns every event with the same correlation_id as the named event,
// in seq order. It is the M0.5 form of `agt why` — useful to see what task
// produced an event. A richer tree-form view lands later.
func (k *Kernel) Why(eventID string) ([]*event.Event, error) {
	var (
		target *event.Event
		all    []*event.Event
	)
	err := k.journal.Range(func(e *event.Event) error {
		all = append(all, e)
		if e.ID == eventID {
			target = e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("runtime: event %s not found", eventID)
	}
	if target.CorrelationID == "" {
		// Singleton event with no correlation — just return it.
		return []*event.Event{target}, nil
	}
	out := make([]*event.Event, 0, 16)
	for _, e := range all {
		if e.CorrelationID == target.CorrelationID {
			out = append(out, e)
		}
	}
	return out, nil
}

// Verify replays every event and confirms the BLAKE3 chain is intact.
// Returns nil on success.
func (k *Kernel) Verify() error {
	return k.journal.Verify()
}
