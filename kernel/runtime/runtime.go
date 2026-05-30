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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
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
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/worldmodel"
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

	// Memory-lite knobs (ROADMAP §2.3). The memory store is always
	// opened and the manager + `agt memory` CLI always work; these
	// flags gate only the per-run behaviour, and all default OFF so the
	// daemon (cmd/agezt) is the single enable point and existing
	// runtime callers/tests are unaffected.
	//
	//   MemoryInject          — recall relevant records and prepend them
	//                           to the System prompt for each run.
	//   MemoryTopK            — max records injected (default 5 when
	//                           MemoryInject and unset).
	//   MemoryTool            — register the in-process `memory` tool so
	//                           the agent can remember/recall/forget.
	//   MemoryDistill         — after a multi-tool run, extract durable
	//                           facts via one best-effort LLM call.
	//   MemoryDistillMinTools — tool-call threshold that triggers
	//                           distillation (default 4 when unset).
	MemoryInject          bool
	MemoryTopK            int
	MemoryTool            bool
	MemoryDistill         bool
	MemoryDistillMinTools int

	// World-model knobs (SPEC-05 §3; Phase 2 slice 1). Like the memory
	// knobs the graph store and `agt world` CLI always work; these flags
	// gate only the per-run behaviour and default OFF (daemon is the single
	// enable point).
	//
	//   WorldInject — resolve entities mentioned in the run's intent and
	//                 prepend a compact "Known entities" block to the System
	//                 prompt (journals worldmodel.retrieved for provenance).
	//   WorldTopK   — max entities injected (default 5 when WorldInject and
	//                 unset).
	//   WorldTool   — register the in-process `world` tool so the agent can
	//                 add/relate/resolve/neighbors during a run.
	WorldInject bool
	WorldTopK   int
	WorldTool   bool

	// Forge / skill knobs (SPEC-05 §4–5; Phase 2 slice 2). The skill store
	// and `agt skill` CLI always work; these gate only the per-run
	// behaviour and default OFF (daemon is the single enable point).
	//
	//   SkillInject        — retrieve matching ACTIVE skills and prepend
	//                        their bodies to the System prompt (journals
	//                        skill.activated for provenance).
	//   SkillTopK          — max skills injected (default 3 when unset).
	//   SkillForge         — after a multi-tool run, propose a DRAFT skill
	//                        via one best-effort LLM call (operator promotes).
	//   SkillForgeMinTools — tool-call threshold that triggers a proposal
	//                        (default 4 when unset).
	SkillInject        bool
	SkillTopK          int
	SkillForge         bool
	SkillForgeMinTools int

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

	memory    *memory.Manager
	memoryDir *memory.FileStore
	world     *worldmodel.Graph
	worldDir  *worldmodel.FileStore
	forge     *skill.Forge
	skillDir  *skill.FileStore
	reflect   *reflect.Engine
	tools     map[string]agent.Tool // cfg.Tools + the memory/world tools (when enabled)

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
	mstore, err := memory.Open(filepath.Join(cfg.BaseDir, "memory"))
	if err != nil {
		j.Close()
		st.Close()
		return nil, fmt.Errorf("runtime: memory: %w", err)
	}
	mgr := memory.NewManager(mstore, kbus)

	wstore, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		return nil, fmt.Errorf("runtime: worldmodel: %w", err)
	}
	wgraph := worldmodel.NewGraph(wstore, kbus)

	skstore, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		return nil, fmt.Errorf("runtime: skills: %w", err)
	}
	forge := skill.NewForge(skstore, kbus)

	// Reflection holds no store of its own — it folds the journal and tunes
	// the world graph, then journals its report (SPEC-05 §6). Default decay
	// knobs; the daemon may override via the optional periodic trigger.
	reflectEng := reflect.New(j, wgraph, kbus, reflect.Config{})

	// The agent's effective tool set is the configured tools plus the
	// in-process memory/world tools (when enabled). Built once, exposed via
	// Tools() so `agt tool list` reflects what the loop actually sees.
	effTools := make(map[string]agent.Tool, len(cfg.Tools)+2)
	maps.Copy(effTools, cfg.Tools)
	if cfg.MemoryTool {
		effTools["memory"] = mgr.Tool()
	}
	if cfg.WorldTool {
		effTools["world"] = wgraph.Tool()
	}

	catStore := catalog.NewStore(catDir)
	cat := cfg.Catalog
	if cat == nil {
		loaded, err := catStore.Load()
		if err != nil {
			j.Close()
			st.Close()
			mstore.Close()
			wstore.Close()
			skstore.Close()
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
		memory:       mgr,
		memoryDir:    mstore,
		world:        wgraph,
		worldDir:     wstore,
		forge:        forge,
		skillDir:     skstore,
		reflect:      reflectEng,
		tools:        effTools,
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
	if err := k.memoryDir.Close(); err != nil {
		return err
	}
	if err := k.worldDir.Close(); err != nil {
		return err
	}
	if err := k.skillDir.Close(); err != nil {
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
// loop sees it — the configured tools plus the in-process `memory`
// tool when MemoryTool is enabled. Read-only — callers must not
// mutate the returned map. Used by the control plane to power
// `agt tool list`, which is operator visibility into what's actually
// wired into the daemon (vs what `agt catalog list` claims about
// providers).
func (k *Kernel) Tools() map[string]agent.Tool { return k.tools }

// Memory returns the memory-lite manager backing `agt memory`, run-time
// context injection, and auto-distillation. Always non-nil after Open.
func (k *Kernel) Memory() *memory.Manager { return k.memory }

// World returns the world-model graph backing `agt world`, run-time entity
// injection, and the Pulse salience relevance signal. Always non-nil after
// Open.
func (k *Kernel) World() *worldmodel.Graph { return k.world }

// Forge returns the skill manager backing `agt skill`, run-time skill
// activation, and post-run skill proposal. Always non-nil after Open.
func (k *Kernel) Forge() *skill.Forge { return k.forge }

// Reflect returns the reflection engine backing `agt reflect` and the optional
// periodic reflection trigger. Always non-nil after Open.
func (k *Kernel) Reflect() *reflect.Engine { return k.reflect }

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

// BaseDir returns the kernel's base directory — the root under
// which journal/, state/, runtime/, catalog/, and vault data
// live. Used by `agt config show` to surface the resolved data
// directory to operators (which can differ from $AGEZT_HOME
// when the daemon was launched with a custom path).
func (k *Kernel) BaseDir() string { return k.cfg.BaseDir }

// Model returns the configured default model name. Empty when
// the daemon uses provider defaults rather than an override.
// Used by `agt config show`.
func (k *Kernel) Model() string { return k.cfg.Model }

// System returns the configured default system prompt. Empty
// when none is set. Used by `agt config show` — but only to
// report PRESENCE, not the prompt content (could contain
// proprietary instructions).
func (k *Kernel) System() string { return k.cfg.System }

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
// `halt` event to the journal so the action is auditable. Equivalent
// to HaltWith("") for callers that have no reason to record.
func (k *Kernel) Halt() { k.HaltWith("") }

// HaltWith is Halt plus a free-text reason that the operator (or
// upstream automation) gave when issuing the halt. The reason is
// journaled on the kernel.halt event so postmortems can answer
// "why was the daemon halted at 14:32?". Empty reason is fine and
// rendered as omitted in the payload.
func (k *Kernel) HaltWith(reason string) {
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
	payload := map[string]any{"cancelled_runs": len(cancels)}
	if reason != "" {
		payload["reason"] = reason
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "kernel.halt",
		Kind:    event.KindHalt,
		Actor:   "kernel",
		Payload: payload,
	})
}

// Resume clears the halt flag, allowing new runs. Already-cancelled runs
// stay cancelled; only future Run calls will succeed. Equivalent to
// ResumeWith("").
func (k *Kernel) Resume() { k.ResumeWith("") }

// ResumeWith is Resume plus a free-text reason recorded on the
// kernel.resume event. Symmetric with HaltWith for postmortem
// reconstruction.
func (k *Kernel) ResumeWith(reason string) {
	k.mu.Lock()
	if !k.halted {
		k.mu.Unlock()
		return
	}
	k.halted = false
	k.mu.Unlock()
	var payload any
	if reason != "" {
		payload = map[string]any{"reason": reason}
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "kernel.resume",
		Kind:    event.KindResume,
		Actor:   "kernel",
		Payload: payload,
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
	// doesn't expose them directly), and so the in-process memory tool
	// can journal its writes under this run.
	runCtx = context.WithValue(runCtx, ctxKeyActor, actor)
	runCtx = context.WithValue(runCtx, ctxKeyCorrelation, corr)
	runCtx = memory.WithCorrelation(runCtx, corr)
	runCtx = worldmodel.WithCorrelation(runCtx, corr)
	runCtx = skill.WithCorrelation(runCtx, corr)

	// Memory injection: recall relevant records and prepend them to the
	// system prompt so the model starts the task already knowing what
	// Agezt remembers. The recall is journaled (memory.retrieved) under
	// corr, so `agt why` shows exactly what knowledge was surfaced.
	system := k.cfg.System
	if k.cfg.MemoryInject {
		topK := k.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		if hits, err := k.memory.Recall(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectMemory(system, hits)
		}
	}

	// World-model injection: resolve the entities the intent refers to and
	// prepend them, so the model starts knowing what "the portfolio" means
	// (SPEC-05 §7 step 1). Resolve journals worldmodel.retrieved under corr,
	// so `agt why` shows what references were grounded.
	if k.cfg.WorldInject {
		topK := k.cfg.WorldTopK
		if topK <= 0 {
			topK = 5
		}
		if hits, err := k.world.Resolve(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectWorld(system, hits)
		}
	}

	// Skill activation: retrieve matching ACTIVE skills and prepend their
	// bodies so the model plans with learned procedures (SPEC-05 §4.2, §7
	// step 4). Activate journals skill.activated under corr for `agt why`.
	if k.cfg.SkillInject {
		topK := k.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		if hits, err := k.forge.Activate(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectSkills(system, hits)
		}
	}

	answer, err := agent.Run(runCtx, agent.LoopConfig{
		Provider:      k.cfg.Provider,
		Tools:         k.tools,
		Bus:           k.bus,
		Model:         k.cfg.Model,
		System:        system,
		MaxIter:       k.cfg.MaxIter,
		Actor:         actor,
		CorrelationID: corr,
		Policy:        k.policyHook,
	}, intent)
	if err != nil {
		return answer, err
	}

	// Auto-distillation: after a multi-tool run, extract durable facts
	// via one best-effort LLM call. Gated on a tool-call threshold so
	// simple Q&A runs aren't taxed with an extra round-trip. Failures are
	// journaled but never propagated — distillation must not turn a
	// successful task into a failed one.
	if k.cfg.MemoryDistill {
		k.maybeDistill(runCtx, corr, intent, answer)
	}

	// Forge proposal: after a multi-tool run, propose a DRAFT skill via one
	// best-effort LLM call (the operator promotes it — §5.1/§5.3). Same
	// threshold-gated, never-fail-the-task contract as distillation.
	if k.cfg.SkillForge {
		k.maybeForge(runCtx, corr, intent, answer)
	}
	return answer, nil
}

// injectMemory prepends a compact "Relevant memory" block to the system
// prompt. Records are rendered one per line as "- [TYPE] subject: content".
func injectMemory(system string, hits []memory.Scored) string {
	var b strings.Builder
	b.WriteString("Relevant memory (recalled from prior tasks; use if helpful):\n")
	for _, h := range hits {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", h.Record.Type, h.Record.Subject, h.Record.Content)
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// injectWorld prepends a compact "Known entities" block to the system prompt.
// Entities are rendered one per line as "- [kind] name (aliases: ...)" so the
// model can ground references like "the portfolio" to concrete things.
func injectWorld(system string, hits []worldmodel.ScoredEntity) string {
	var b strings.Builder
	b.WriteString("Known entities (from the world model; use to ground references):\n")
	for _, h := range hits {
		e := h.Entity
		if len(e.Aliases) > 0 {
			fmt.Fprintf(&b, "- [%s] %s (aka %s)\n", e.Kind, e.Name, strings.Join(e.Aliases, ", "))
		} else {
			fmt.Fprintf(&b, "- [%s] %s\n", e.Kind, e.Name)
		}
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// injectSkills prepends matching active skills' bodies to the system prompt so
// the model plans with learned procedures. Each is rendered as a titled block.
func injectSkills(system string, hits []skill.Scored) string {
	var b strings.Builder
	b.WriteString("Applicable skills (learned procedures; follow if relevant):\n")
	for _, h := range hits {
		s := h.Skill
		fmt.Fprintf(&b, "## %s — %s\n%s\n", s.Name, s.Description, s.Body)
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// maybeForge folds the run's journal by correlation, and if the run made at
// least SkillForgeMinTools tool calls, runs one best-effort skill proposal over
// a compact transcript. Best-effort: any error is journaled and swallowed — a
// proposal must never turn a successful task into a failed one.
func (k *Kernel) maybeForge(ctx context.Context, corr, intent, answer string) {
	minTools := k.cfg.SkillForgeMinTools
	if minTools <= 0 {
		minTools = 4
	}
	toolCount, names := k.foldRunTools(corr)
	if toolCount < minTools {
		return
	}
	transcript := buildTranscript(names, answer)
	if _, err := k.forge.Propose(ctx, corr, k.cfg.Provider, k.cfg.Model, intent, transcript); err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject:       "skill.propose_failed",
			Kind:          event.KindSkillCreated,
			Actor:         "forge",
			CorrelationID: corr,
			Payload:       map[string]any{"action": "propose_failed", "error": err.Error()},
		})
	}
}

// maybeDistill folds the run's journal by correlation, and if the run made at
// least MemoryDistillMinTools tool calls, runs one best-effort distillation
// pass over a compact transcript. Best-effort: any error is journaled as a
// memory distill failure and swallowed.
func (k *Kernel) maybeDistill(ctx context.Context, corr, intent, answer string) {
	minTools := k.cfg.MemoryDistillMinTools
	if minTools <= 0 {
		minTools = 4
	}
	toolCount, names := k.foldRunTools(corr)
	if toolCount < minTools {
		return
	}
	transcript := buildTranscript(names, answer)
	if _, err := k.memory.Distill(ctx, corr, k.cfg.Provider, k.cfg.Model, intent, transcript); err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject:       "memory.distill_failed",
			Kind:          event.KindMemoryWritten,
			Actor:         "memory",
			CorrelationID: corr,
			Payload:       map[string]any{"action": "distill_failed", "error": err.Error()},
		})
	}
}

// foldRunTools counts tool.result events for corr and collects the tool names
// invoked (in order), for the distillation transcript.
func (k *Kernel) foldRunTools(corr string) (int, []string) {
	var (
		count int
		names []string
	)
	_ = k.journal.Range(func(e *event.Event) error {
		if e.CorrelationID != corr || e.Kind != event.KindToolResult {
			return nil
		}
		count++
		var p struct {
			Tool string `json:"tool"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Tool != "" {
			names = append(names, p.Tool)
		}
		return nil
	})
	return count, names
}

// buildTranscript renders a compact, token-cheap summary of a run for the
// distiller: the tools used and the final answer.
func buildTranscript(toolNames []string, answer string) string {
	var b strings.Builder
	if len(toolNames) > 0 {
		b.WriteString("Tools used: ")
		b.WriteString(strings.Join(toolNames, ", "))
		b.WriteString("\n")
	}
	b.WriteString("Final answer:\n")
	b.WriteString(answer)
	return b.String()
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
