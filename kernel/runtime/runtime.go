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
	stdruntime "runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/tenantctx"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workflow"
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

	// TenantID is the id of the tenant this kernel serves, or "" for the primary
	// (non-multi-tenant) kernel. When non-empty it is stamped onto every run's context
	// (via tenantctx in RunWith) so tenant-aware tools — e.g. the mesh remote_run tool
	// selecting a per-tenant peer set — can discover which tenant they are serving,
	// regardless of whether the run was triggered over HTTP, a schedule, or a channel.
	TenantID string

	// Provider is the LLM provider the agent loop will drive.
	Provider agent.Provider

	// Tools are the in-process tools advertised to the model.
	Tools map[string]agent.Tool

	// ScriptRunner executes forged script tools (M794) in the code-exec
	// sandbox. When set, every run is additionally offered the toolforge
	// store's ACTIVE scripts as callable `forge_<name>` tools; nil disables
	// the offering (drafting/testing then reports the forge unavailable).
	// The daemon wires the code_exec tool here — same warden isolation,
	// scrubbed env, and `code.exec` Edict gate as direct code execution.
	ScriptRunner toolforge.Runner

	// MCPDialer spawns + handshakes one MCP server on attach (M796). Nil
	// means the production stdio dialer (mcp.Dial); tests inject fakes.
	MCPDialer mcp.Dialer

	// Model is the default model name passed to the provider.
	Model string

	// System is the system prompt prepended to every run.
	System string

	// MaxIter caps tool-call rounds per run (DECISIONS E5).
	MaxIter int

	// MaxAutoContinue caps how many times a run that exhausts MaxIter without a
	// final answer is automatically continued (M833) before failing with
	// max_iters. 0 → the agent loop's default; negative → disabled. Passed
	// straight through to LoopConfig.
	MaxAutoContinue int

	// AutoContinueWait is the breather before each automatic continuation (M833).
	// 0 → the loop's default. Passed straight through to LoopConfig.
	AutoContinueWait time.Duration

	// MaxDuration is an optional per-run wall-clock budget (M31). When > 0,
	// RunWith wraps the run context with this deadline; a run that overruns
	// is cancelled and the agent loop returns context.DeadlineExceeded,
	// which the M30 terminal emitter classifies as task.failed(reason=
	// timeout). 0 (the default) means no wall-clock cap — only MaxIter and
	// explicit halt bound a run. Distinct from a halt: the deadline cancels
	// with DeadlineExceeded, while Halt() cancels with Canceled, so the two
	// stay distinguishable in the failure reason.
	MaxDuration time.Duration

	// ToolTimeout is an optional per-tool-call wall-clock budget (M34),
	// passed straight through to the agent loop's LoopConfig. When > 0, a
	// single tool invocation that overruns is cancelled and the model is
	// handed an error result — the run continues, unlike MaxDuration which
	// fails the whole run. 0 (the default) means no per-tool cap.
	ToolTimeout time.Duration

	// MaxParallelTools caps how many tool calls from one assistant turn run
	// concurrently (M880), passed straight through to LoopConfig. 0 → the
	// agent loop's default; 1 or negative → strictly sequential.
	MaxParallelTools int

	// ShutdownDrainTimeout bounds how long Close waits for in-flight runs
	// (and async delegations) to settle after Halt cancels them, BEFORE the
	// journal/state/memory stores they write to are torn down (M883). A run
	// blocked in a tool that ignores cancellation no longer races store
	// teardown — it gets this grace window, then Close proceeds anyway.
	// 0 → DefaultShutdownDrainTimeout; negative → no wait (the historical
	// immediate teardown).
	ShutdownDrainTimeout time.Duration

	// SubAgentTool registers the in-process `delegate` tool (P6-MULTI-01) so
	// a lead agent can spawn a bounded sub-agent for a focused subtask and get
	// back its summary. Off by default; the daemon is the single enable point.
	SubAgentTool bool
	// SubAgentMaxDepth bounds how deep delegation can nest (a sub-agent calling
	// delegate again). Defaults to 1 when SubAgentTool is on and this is unset
	// — one level of sub-agents, no unbounded recursion.
	SubAgentMaxDepth int
	// SubAgentMaxFanout bounds how many sub-agents a SINGLE agent run may spawn
	// at its level (depth caps nesting; fan-out caps breadth). The Nth+1
	// delegate call from one run is refused with a tool error the lead adapts
	// to. 0 (the default) means unbounded — the historical behaviour; the
	// daemon is the single enable point.
	SubAgentMaxFanout int
	// SubAgentMaxSpendMicrocents caps the TOTAL spend (in microcents) a single
	// run's sub-agents may collectively consume. Once a lead's delegations have
	// spent past this, the next delegate is refused — the cost analogue of
	// SubAgentMaxFanout's count cap (M48), closing the count→cost→cap loop atop
	// M47's per-delegation spend attribution. Read from the journal (durable by
	// the time each child returns), so it needs no in-memory tally. 0 (the
	// default) means unbounded; the daemon is the single enable point.
	SubAgentMaxSpendMicrocents int64
	// SubAgentMaxTotal caps the TOTAL number of sub-agents in one delegation
	// TREE — every descendant of a root run summed across all depths, not just
	// one spawner's breadth (SubAgentMaxFanout) or one lead's direct children.
	// This is the rail that makes depth>1 healthy: with depth D and fan-out F a
	// tree can hold up to F^D leaves, so a per-spawner fan-out cap alone doesn't
	// bound the whole tree's size. The (N+1)th spawn ANYWHERE in the tree is
	// refused with a tool error the spawning agent adapts to. Counted in-memory
	// per root correlation, released when the root run ends. 0 (the default)
	// means unbounded; the daemon is the single enable point. (M629)
	SubAgentMaxTotal int

	// Edict is the policy engine that gates each tool call. If nil, a
	// default engine (edict.New(edict.Options{})) is constructed — the
	// runtime is never policy-less.
	Edict *edict.Engine

	// ToolCapabilities maps tool names (as registered, i.e. prefixed for
	// plugin tools) to a DECLARED Edict capability (M900) — the kernel-side
	// half of the plugin capability manifest. A mapped tool is classified
	// under the declared axis (its trust level + hard-deny rules) instead of
	// the unknown-capability default. Declarations naming a capability the
	// kernel doesn't know are dropped at Open — plugins join existing axes,
	// they don't invent them. Nil/empty = historical classification only.
	ToolCapabilities map[string]string

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

	// ApprovalTimeout overrides how long a HITL approval blocks waiting
	// for an operator before it auto-denies (DecisionTimeout). Zero means
	// approval.DefaultTimeout (5m). Only applied when Approvals is nil and
	// the kernel constructs the default registry (M100); an explicitly
	// supplied registry carries its own timeout.
	ApprovalTimeout time.Duration

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
	// MemoryEmbedder, when non-nil, upgrades memory recall from the local
	// feature-hash embedding to true provider embeddings (M884, DECISIONS C5
	// opt-in). The kernel never picks an implementation — the daemon injects
	// one (typically backed by a provider plugin). Recall falls back to the
	// local hybrid on any embedder failure.
	MemoryEmbedder memory.Embedder

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
	// ShadowEval, when true, judges the shadow skills relevant to a completed run
	// against what actually happened (SPEC-05 §5.2): an opt-in, best-effort LLM
	// judgement per relevant shadow skill — it executes nothing, so it cannot
	// affect outcomes — recorded as shadow_evals/shadow_wins for the (M401)
	// shadow→active promotion gate. Off by default (it spends extra provider
	// calls). Only meaningful when SkillForge/skills are in use.
	ShadowEval bool

	// EnvironmentInject, when true, prepends a concise "runtime environment"
	// preamble to the system prompt for every run (M609): the host OS/arch, the
	// shell the shell tool uses, the shared workspace directory, today's date,
	// and the available tools. Without it the model flies blind about its host —
	// e.g. it tries `ls`/`cat` on a Windows box where the shell is `cmd`, burning
	// iterations on "not recognized" errors before adapting. The preamble is
	// derived fresh per run (cfg.Now) so the date is always current.
	EnvironmentInject bool
	// WorkspaceRoot is the absolute directory the file and shell tools both
	// operate in. Surfaced to the model by the environment preamble so it
	// references the right path. Empty omits the workspace line.
	WorkspaceRoot string

	// ArtifactThreshold is the tool-output byte size above which the agent loop
	// offloads the output to the content-addressed artifact store and journals a
	// raw_ref + preview instead of the full bytes (SPEC-04 §3.6 / SPEC-01 §10.2).
	// 0 uses agent.DefaultArtifactThreshold.
	ArtifactThreshold int

	// ContextBudget caps the assembled-context size (chars) the agent loop sends
	// per provider call (SPEC-10 §3); when exceeded the loop elides the oldest
	// tool outputs and journals context.compacted. 0 disables (full history).
	ContextBudget int
	// ContextBudgetAuto, when true and ContextBudget is 0, derives a per-run
	// budget from the resolved model's catalog context window (half the window,
	// ~4 chars/token). An unknown model leaves compaction off. An explicit
	// ContextBudget always wins. (M394)
	ContextBudgetAuto bool
	// ContextProtectFirst is how many of the earliest messages context compaction
	// never elides, preserving the run's original grounding. 0 keeps the default
	// oldest-first behaviour (only the tail is shielded). (M395)
	ContextProtectFirst int
	// ContextSummarize, when true, replaces the deterministic head-snippet stub of
	// an elided tool output with a one-line abstractive summary produced by a
	// bounded provider call (M398). Off by default — it spends extra (cached,
	// once-per-output) provider calls, so the operator opts in. Only meaningful
	// when context compaction is active (ContextBudget/Auto set).
	ContextSummarize bool

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

	// VisionModel, when set, returns a vision-capable model id the governor can
	// route to (among the registered+credentialed providers), or ("", false) if
	// none is keyed. Injected by the daemon (cmd/agezt) which owns the registered
	// set. Used by DescribeImages (M821) to caption images for a run whose active
	// model can't see them. Nil disables the vision sidecar.
	VisionModel func() (modelID string, ok bool)

	// ModelAvailable, when set, reports whether a model id can actually be served
	// by a registered+credentialed provider. The daemon (cmd/agezt) injects it
	// (it owns the keyed set). Delegation uses it to drop unkeyed models from a
	// sub-agent's model chain (M838 bugfix) so a delegate never runs on a provider
	// with no API key. Nil → no filtering (the historical behaviour; tests).
	ModelAvailable func(modelID string) bool

	// CouncilMembers, when set, returns the default Council of Elders membership
	// (M837) — one seat per keyed provider's best model, so the council speaks
	// across providers. Injected by the daemon (cmd/agezt), which owns the
	// registered+credentialed set and the AGEZT_COUNCIL_MEMBERS override. Nil or
	// empty → the council tool reports no members available.
	CouncilMembers func() []CouncilMember
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
	standing  *standing.Store
	roster    *roster.Store
	toolForge *toolforge.Store
	mcpStore  *mcp.Store
	workflows *workflow.Store
	artifacts *artifact.Store
	artIndex  *artifact.Index // metadata sidecar over artifacts (M822): browsable/deletable entries
	lake      *datalake.Lake  // Personal Data Lake (M834): agent-built structured collections
	reflect   *reflect.Engine
	schedules *cadence.Store        // persistent scheduled-intents store (autonomy)
	tools     map[string]agent.Tool // cfg.Tools + the memory/world tools (when enabled)

	catalogStore *catalog.Store
	catalog      *catalog.Catalog // snapshot — refreshable via ReloadCatalog

	mu     sync.Mutex
	halted bool
	system string                        // live agent persona / system prompt (M710); seeded from cfg.System, editable at runtime
	model  string                        // live default model id (M816); seeded from cfg.Model, hot-swapped on provider reload
	runs   map[string]context.CancelFunc // correlation_id → cancel
	fanout map[string]int                // spawning correlation_id → sub-agents spawned (M46 fan-out bound)
	tree   map[string]int                // root correlation_id → total sub-agents in the tree (M629 total bound)
	steers map[string]*runControl        // correlation_id → live-steering control surface (M608)
	spawns map[string]*spawnHandle       // child correlation_id → pending/finished async delegation (M881)
	runWG  sync.WaitGroup                // in-flight runs + async spawn goroutines; Close drains it bounded (M883)

	// toolCaps is the validated declared-capability overlay (M900): tool name
	// → Edict capability, consulted by policyHook before the built-in
	// classification. Built once at Open from cfg.ToolCapabilities (known
	// capabilities only); read-only afterwards, so no lock needed.
	toolCaps map[string]edict.Capability
	// mcpConns are the LIVE MCP attachments (M796): server name → connection.
	// Merged into every run's tool map (mergeMCPTools); detach removes.
	mcpConns map[string]mcp.Conn

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
		apr = approval.New(approval.Config{Bus: kbus, Timeout: cfg.ApprovalTimeout})
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
	if cfg.MemoryEmbedder != nil {
		mgr.SetEmbedder(cfg.MemoryEmbedder) // M884: provider embeddings opt-in
	}

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
	// Wire the on-disk bundle store so skills can ship reference files + scripts
	// (agentskills.io shape, M847). Best-effort: a bundle-store failure leaves
	// skills body-only rather than failing daemon start.
	if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
		forge.SetBundles(bundles)
	}

	schedStore, err := cadence.OpenStore(filepath.Join(cfg.BaseDir, "cadence"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: cadence: %w", err)
	}

	// Content-addressed artifact store (SPEC-04 §3.6): the agent loop offloads
	// oversized tool outputs here so the journal stays small. Store-only — no bus.
	artStore, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: artifacts: %w", err)
	}
	// Metadata index over the blob store (M822) — browsable/deletable entries
	// (inbound images, tool outputs). Failure here is non-fatal to the blob store
	// but we surface it so the operator knows the file-manager won't populate.
	artIndex, err := artifact.OpenIndex(artStore, filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: artifact index: %w", err)
	}

	// Personal Data Lake (M834): file-based structured collections agents build
	// and share. Pure on-disk (no handle to close), so its error path just unwinds
	// the prior stores like the others.
	lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: data lake: %w", err)
	}
	// Seed the built-in Personal Data Lake collections (M835) — expenses, calendar,
	// tasks, notes, habits, bookmarks, contacts. Idempotent (EnsureCollection skips
	// existing ones) and best-effort: a seed hiccup must not block boot, and the
	// next start retries.
	_, _ = lake.SeedBuiltins("system")

	ststore, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: standing: %w", err)
	}

	rstore, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: roster: %w", err)
	}

	tfstore, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: toolforge: %w", err)
	}

	mcpstore, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: mcp: %w", err)
	}

	wfstore, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, fmt.Errorf("runtime: workflows: %w", err)
	}

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
	// The sub-agent tool's runner needs the finished *Kernel, which doesn't
	// exist yet; register the tool now and wire its runner just after k is
	// built (effTools is the same map k.tools holds).
	var subTool *subAgentTool
	var awaitTool *subAgentAwaitTool
	if cfg.SubAgentTool {
		subTool = newSubAgentTool()
		effTools["delegate"] = subTool
		// The collect half of async delegation (M881): delegate(async=true)
		// returns a spawn_id; delegate_await blocks until that child finishes.
		awaitTool = newSubAgentAwaitTool()
		effTools["delegate_await"] = awaitTool
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
		standing:     ststore,
		roster:       rstore,
		toolForge:    tfstore,
		mcpStore:     mcpstore,
		mcpConns:     make(map[string]mcp.Conn),
		workflows:    wfstore,
		artifacts:    artStore,
		artIndex:     artIndex,
		lake:         lake,
		reflect:      reflectEng,
		schedules:    schedStore,
		tools:        effTools,
		system:       cfg.System,
		model:        cfg.Model,
		runs:         make(map[string]context.CancelFunc),
		fanout:       make(map[string]int),
		tree:         make(map[string]int),
		steers:       make(map[string]*runControl),
		spawns:       make(map[string]*spawnHandle),
		toolCaps:     validatedToolCaps(cfg.ToolCapabilities), // M900
		startTime:    time.Now(),
	}
	if subTool != nil {
		subTool.run = k.runSubAgent
		subTool.spawn = k.runSubAgentAsync // M881: non-blocking delegation
	}
	if awaitTool != nil {
		awaitTool.await = k.awaitSubAgent
	}
	return k, nil
}

// DefaultShutdownDrainTimeout is how long Close waits for cancelled in-flight
// runs to actually return before tearing down their stores (M883).
const DefaultShutdownDrainTimeout = 5 * time.Second

// Close stops the bus, then closes state and the journal. Pending runs are
// cancelled via Halt, then given a bounded drain window (M883) so a run
// mid-journal-write finishes cleanly instead of racing store teardown.
func (k *Kernel) Close() error {
	k.Halt() // cancel any in-flight runs first
	// Drain: cancelled runs still need to unwind — publish their terminal
	// task.failed, release fan-out tallies, return from tools that honour the
	// cancel late. Wait bounded; a run wedged in a cancel-ignoring tool must
	// not block shutdown forever.
	drain := k.cfg.ShutdownDrainTimeout
	if drain == 0 {
		drain = DefaultShutdownDrainTimeout
	}
	if drain > 0 {
		settled := make(chan struct{})
		go func() {
			k.runWG.Wait()
			close(settled)
		}()
		t := time.NewTimer(drain)
		select {
		case <-settled:
			t.Stop()
		case <-t.C:
			// Best-effort breadcrumb: the journal is still open here, so the
			// abandonment is auditable. The wedged goroutine dies with the
			// process.
			_, _ = k.bus.Publish(event.Spec{
				Subject: "kernel.shutdown",
				Kind:    event.KindAnomalyDetected,
				Actor:   "kernel",
				Payload: map[string]any{
					"anomaly":  "shutdown_drain_timeout",
					"waited":   drain.String(),
					"detail":   "in-flight runs did not settle after Halt; closing stores anyway",
					"severity": "warning",
				},
			})
		}
	}
	k.closeMCPConns() // detach every live MCP server (kills the children)
	k.bus.Close()
	// Close every store even if an earlier one errors — the previous short-circuit
	// returned on the first error and leaked the remaining handles, notably the
	// journal's OS file descriptor (a held handle blocks a re-Open of the dir on
	// Windows). errors.Join reports all failures. (M477)
	return closeAll(
		k.state.Close,
		k.memoryDir.Close,
		k.worldDir.Close,
		k.skillDir.Close,
		k.journal.Close,
	)
}

// closeAll invokes every close func (none skipped) and joins their errors.
func closeAll(closers ...func() error) error {
	errs := make([]error, 0, len(closers))
	for _, c := range closers {
		errs = append(errs, c())
	}
	return errors.Join(errs...)
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

// Schedules returns the persistent scheduled-intents store (autonomy). The
// cadence resident fires its due entries; `agt schedule` manages them.
func (k *Kernel) Schedules() *cadence.Store { return k.schedules }

// World returns the world-model graph backing `agt world`, run-time entity
// injection, and the Pulse salience relevance signal. Always non-nil after
// Open.
func (k *Kernel) World() *worldmodel.Graph { return k.world }

// Forge returns the skill manager backing `agt skill`, run-time skill
// activation, and post-run skill proposal. Always non-nil after Open.
func (k *Kernel) Forge() *skill.Forge { return k.forge }

// Artifacts returns the content-addressed artifact store (SPEC-04 §3.6), where
// the loop offloads oversized tool outputs. Used by retrieval surfaces.
func (k *Kernel) Artifacts() *artifact.Store { return k.artifacts }

// ArtifactIndex returns the metadata index over the blob store (M822) — the
// browsable/deletable per-arrival entries (inbound images, tool outputs) the
// file manager and inbound-image persistence use.
func (k *Kernel) ArtifactIndex() *artifact.Index { return k.artIndex }

// DataLake returns the Personal Data Lake (M834) — the file-based structured
// collections agents build and share, surfaced by the `db` tool and the Web UI.
func (k *Kernel) DataLake() *datalake.Lake { return k.lake }

// Standing returns the Chronos standing-order store (SPEC-16 §4), backing
// `agt standing`. Always non-nil after Open.
func (k *Kernel) Standing() *standing.Store { return k.standing }

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (k *Kernel) AddStanding(o standing.Order) (standing.Order, error) {
	saved, err := k.standing.Add(o)
	if err != nil {
		return standing.Order{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling standing.updated.
func (k *Kernel) SetStandingEnabled(id string, enabled bool) (standing.Order, error) {
	o, err := k.standing.SetEnabled(id, enabled)
	if err != nil {
		return standing.Order{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "enabled": enabled, "action": state},
	})
	return o, nil
}

// UpdateStanding edits a standing order's mutable fields via mutate, journaling
// standing.updated (action "edited") on success. Identity/lifecycle fields are
// protected by the store. Returns the updated order and whether the id existed
// (false + nil error for an unknown id, mirroring the schedule-edit path).
func (k *Kernel) UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error) {
	o, err := k.standing.Update(id, mutate)
	if errors.Is(err, standing.ErrNotFound) {
		return standing.Order{}, false, nil
	}
	if err != nil {
		return standing.Order{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "action": "edited"},
	})
	return o, true, nil
}

// RemoveStanding deletes a standing order, journaling standing.removed when it
// existed. Returns whether it existed.
func (k *Kernel) RemoveStanding(id string) (bool, error) {
	o, _ := k.standing.Get(id)
	ok, err := k.standing.Remove(id)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "standing." + id, Kind: event.KindStandingRemoved, Actor: "standing",
			Payload: map[string]any{"id": id, "name": o.Name},
		})
	}
	return ok, nil
}

// Roster returns the durable agent-profile store (M783). Always non-nil after Open.
func (k *Kernel) Roster() *roster.Store { return k.roster }

// AddProfile validates and persists a named agent profile, journaling
// roster.created so the agent's birth is auditable.
func (k *Kernel) AddProfile(p roster.Profile) (roster.Profile, error) {
	saved, err := k.roster.Add(p)
	if err != nil {
		return roster.Profile{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + saved.Slug, Kind: event.KindRosterCreated, Actor: "roster",
		Payload: map[string]any{"id": saved.ID, "slug": saved.Slug, "name": saved.Name, "model": saved.Model},
	})
	return saved, nil
}

// SetProfileEnabled pauses/resumes an agent profile, journaling roster.updated.
func (k *Kernel) SetProfileEnabled(ref string, enabled bool) (roster.Profile, error) {
	p, err := k.roster.SetEnabled(ref, enabled)
	if err != nil {
		return roster.Profile{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "enabled": enabled, "action": state},
	})
	return p, nil
}

// SetProfileRetired moves an agent to the graveyard (true) or revives it (false)
// by ref, journaling roster.updated. Retiring also pauses the agent so it stops
// firing (M846). A graveyard agent is excluded from delegation (runSubAgent).
func (k *Kernel) SetProfileRetired(ref string, retired bool) (roster.Profile, error) {
	p, err := k.roster.SetRetired(ref, retired)
	if err != nil {
		return roster.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "action": action},
	})
	return p, nil
}

// AgentImpact reports what depends on an agent before it is retired/removed
// (M846) — the standing orders that fire AS it. The operator sees this in the
// retire confirmation so the "etkileri" are explicit, not a surprise. Returns the
// affected orders as "name (id)" strings, or nil when nothing references it.
func (k *Kernel) AgentImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" || k.standing == nil {
		return nil
	}
	var out []string
	for _, o := range k.standing.List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			name := o.Name
			if name == "" {
				name = o.ID
			}
			out = append(out, fmt.Sprintf("%s (%s)", name, o.ID))
		}
	}
	return out
}

// UpdateProfile edits a profile's mutable fields via mutate, journaling
// roster.updated (action "edited"). Identity/lifecycle fields are protected by
// the store. Returns false + nil error for an unknown ref (standing pattern).
func (k *Kernel) UpdateProfile(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	p, err := k.roster.Update(ref, mutate)
	if errors.Is(err, roster.ErrNotFound) {
		return roster.Profile{}, false, nil
	}
	if err != nil {
		return roster.Profile{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "action": "edited"},
	})
	return p, true, nil
}

// RemoveProfile deletes an agent profile, journaling roster.removed when it
// existed. Returns whether it existed.
func (k *Kernel) RemoveProfile(ref string) (bool, error) {
	gone, ok, err := k.roster.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "roster." + gone.Slug, Kind: event.KindRosterRemoved, Actor: "roster",
			Payload: map[string]any{"id": gone.ID, "slug": gone.Slug, "name": gone.Name},
		})
	}
	return ok, nil
}

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

// ActiveRunIDs returns the correlation ids of the runs in flight right now —
// the live keys of the cancel registry, sorted for determinism. This is the
// "what is running" the overseer (M850) cancels by id, distinct from the
// journal-derived run history (CmdRunsList): these are exactly the runs a
// CancelRun can still stop. Safe under concurrent run starts/completes.
func (k *Kernel) ActiveRunIDs() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	ids := make([]string, 0, len(k.runs))
	for corr := range k.runs {
		ids = append(ids, corr)
	}
	slices.Sort(ids)
	return ids
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

// Model returns the live default model name. Empty when the daemon uses
// provider defaults rather than an override. Seeded from cfg.Model at Open and
// hot-swapped via SetModel when the provider is reloaded (M816), so it must be
// mu-guarded like the persona. Used by `agt config show` and every run that
// builds a CompletionRequest without an explicit per-run/per-task model.
func (k *Kernel) Model() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.model
}

// SetModel replaces the live default model id. The next run picks it up — no
// restart. Paired with SetSystem-style persistence: the daemon's provider
// reload calls this after AGEZT_MODEL changes so a wizard/Config-Center edit
// takes effect in place instead of waiting for the next boot (M816).
func (k *Kernel) SetModel(m string) {
	k.mu.Lock()
	k.model = m
	k.mu.Unlock()
}

// MaxDuration is the daemon-wide per-run wall-clock budget (M31), 0 if disabled.
// Exposed so the control plane can report the effective timeout in `agt run
// --dry-run` (M159) without reaching into the config.
func (k *Kernel) MaxDuration() time.Duration { return k.cfg.MaxDuration }

// SubAgentLimits reports the active delegation-governance ceilings (M46–M48)
// for `agt status` (M49). Enabled mirrors whether the `delegate` tool is
// registered; MaxDepth is the EFFECTIVE cap (defaulting to 1 when enabled and
// unset, exactly as runSubAgent does); MaxFanout / MaxSpendMicrocents of 0 mean
// unbounded. Read-only — surfaces config the operator set, makes silent
// governance legible.
type SubAgentLimits struct {
	Enabled            bool
	MaxDepth           int
	MaxFanout          int
	MaxSpendMicrocents int64
	MaxTotal           int
}

// SubAgentLimits returns the effective delegation ceilings (M49).
func (k *Kernel) SubAgentLimits() SubAgentLimits {
	l := SubAgentLimits{
		Enabled:            k.cfg.SubAgentTool,
		MaxDepth:           k.cfg.SubAgentMaxDepth,
		MaxFanout:          k.cfg.SubAgentMaxFanout,
		MaxSpendMicrocents: k.cfg.SubAgentMaxSpendMicrocents,
		MaxTotal:           k.cfg.SubAgentMaxTotal,
	}
	if l.Enabled && l.MaxDepth <= 0 {
		l.MaxDepth = 1 // effective default, matching runSubAgent
	}
	return l
}

// System returns the live default system prompt (agent persona). Empty when none
// is set. Seeded from cfg.System at Open and editable at runtime via SetSystem
// (M710). `agt config show` uses it only to report PRESENCE, not content (which
// could carry proprietary instructions); the dedicated persona surface returns
// the content for the owner to edit.
func (k *Kernel) System() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.system
}

// SetSystem replaces the live default system prompt (agent persona). The next run
// picks it up — no restart. Persistence (so it survives a restart) is the control
// plane's job: it writes AGEZT_SYSTEM_PROMPT to the config store alongside this.
func (k *Kernel) SetSystem(s string) {
	k.mu.Lock()
	k.system = s
	k.mu.Unlock()
}

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
// validatedToolCaps keeps only declarations naming a capability Edict knows
// (M900) — a plugin may join an existing policy axis, never invent one.
func validatedToolCaps(declared map[string]string) map[string]edict.Capability {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]edict.Capability, len(declared))
	for tool, cap := range declared {
		if edict.KnownCapability(cap) {
			out[tool] = edict.Capability(cap)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (k *Kernel) policyHook(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
	// Declared-capability overlay (M900): a plugin tool whose manifest joined
	// a known policy axis is classified there; everything else falls through
	// to the built-in name/input classification.
	cap, declared := k.toolCaps[tc.Name]
	if !declared {
		cap = edict.CapabilityForToolCall(tc.Name, tc.Input)
	}
	var out edict.Outcome
	if ceiling, ok := trustCeilingFromCtx(ctx); ok {
		out = k.edict.DecideWithCeiling(cap, string(tc.Input), ceiling) // SPEC-16 §4 initiative ceiling
	} else {
		out = k.edict.Decide(cap, string(tc.Input))
	}

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
	ctxKeyModel
	ctxKeyImages
	ctxKeySystem
	ctxKeyRunTimeout
	ctxKeyTools
	ctxKeyMaxCost
	ctxKeyJSONMode
	ctxKeyTrustCeiling
	ctxKeyRoot
	ctxKeyModelChain
	ctxKeyAgentIdent
)

// agentIdent carries a named agent's identity + daily ceiling for the
// Governor's per-agent ledger (M793).
type agentIdent struct {
	slug    string
	dailyMc int64
}

// rootFromCtx returns the correlation of the ROOT run of a delegation tree (the
// top-level lead), propagated to every descendant so a tree-wide cap can be
// attributed to the whole tree rather than a single spawner. Empty when not in
// a delegated context.
func rootFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRoot).(string); ok {
		return v
	}
	return ""
}

// WithTrustCeiling returns a context capping autonomous tool-use at `ceiling` for
// the run started with it (SPEC-16 §4 initiative.max_trust). The policy hook
// consults it so a normally auto-allowed capability is downgraded to Ask (or
// Deny) within this run. ceiling >= LevelAllow is a no-op (no clamp). Used by the
// Chronos standing-order runner to bound an order's autonomy.
func WithTrustCeiling(ctx context.Context, ceiling edict.TrustLevel) context.Context {
	if ceiling >= edict.LevelAllow {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling)
}

func trustCeilingFromCtx(ctx context.Context) (edict.TrustLevel, bool) {
	v, ok := ctx.Value(ctxKeyTrustCeiling).(edict.TrustLevel)
	return v, ok
}

// WithImages returns a context carrying image-attachment references for the run
// started with it (M93). They flow into the agent loop's initial user message.
// Empty is a no-op. The caller (control plane) only sets this after the M91
// vision gate confirms the active model is vision-capable.
func WithImages(ctx context.Context, images []string) context.Context {
	if len(images) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyImages, images)
}

func imagesFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxKeyImages).([]string); ok {
		return v
	}
	return nil
}

// WithJSONMode returns a context requesting structured (JSON) output for the run
// started with it (M314). It flows into the agent loop's CompletionRequest.JSONMode,
// so a provider with a native JSON mode constrains its output. false is a no-op.
// Used by the OpenAI-compatible API to honour a client's response_format.
func WithJSONMode(ctx context.Context, jsonMode bool) context.Context {
	if !jsonMode {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyJSONMode, true)
}

func jsonModeFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyJSONMode).(bool)
	return v
}

// WithModel returns a context that overrides the model for the run started with
// it. Empty model is a no-op (the kernel's configured Model is used). The
// override flows into the agent loop's CompletionRequest.Model, so the selected
// provider serves exactly the requested model — the basis for per-request model
// selection from the OpenAI-compatible API.
func WithModel(ctx context.Context, model string) context.Context {
	if model == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyModel, model)
}

func modelFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyModel).(string); ok {
		return v
	}
	return ""
}

// WithSystem returns a context that overrides the base system prompt for the run
// started with it (M148-sibling). Empty is a no-op (the kernel's configured System
// is used). The override REPLACES the configured System; memory/world/skill
// injection still layer on top, so a one-off persona/instruction can be set per run
// without losing what Agezt knows.
func WithSystem(ctx context.Context, system string) context.Context {
	if system == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeySystem, system)
}

func systemFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeySystem).(string); ok {
		return v
	}
	return ""
}

// WithRunTimeout returns a context that overrides the per-run wall-clock budget
// for the run started with it (a per-run counterpart to Config.MaxDuration / M31).
// d <= 0 is a no-op (the configured MaxDuration, if any, applies). Lets a single
// run be bounded without a daemon-wide cap (`agt run --timeout`).
func WithRunTimeout(ctx context.Context, d time.Duration) context.Context {
	if d <= 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRunTimeout, d)
}

func runTimeoutFromCtx(ctx context.Context) time.Duration {
	if v, ok := ctx.Value(ctxKeyRunTimeout).(time.Duration); ok {
		return v
	}
	return 0
}

// WithMaxCost returns a context that caps the cumulative provider spend (in
// USD-microcents) for the run started with it (M166) — the per-run cost analogue
// of WithRunTimeout. mc <= 0 is a no-op (uncapped). Lets a single run be bounded
// by money (`agt run --max-cost`) without a daemon-wide ceiling; the Governor's
// daily ceiling still applies on top.
func WithMaxCost(ctx context.Context, mc int64) context.Context {
	if mc <= 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyMaxCost, mc)
}

func maxCostFromCtx(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxKeyMaxCost).(int64); ok {
		return v
	}
	return 0
}

// WithAgentProfile applies a roster profile to a run's context (M790): the
// soul becomes the system override, the model + ordered fallbacks become the
// run's model chain, and the memory scope follows the identity (M786). The
// per-run cost ceiling is NOT applied here — callers layer it so their own
// explicit budget wins (mirrors handleRun's precedence). Used by the standing
// runner so an order can fire AS a named agent; handleRun keeps its inline
// application (its model resolves before the vision gate).
func WithAgentProfile(ctx context.Context, p roster.Profile) context.Context {
	if soul := strings.TrimSpace(p.Soul); soul != "" {
		ctx = WithSystem(ctx, soul)
	}
	primary := strings.TrimSpace(p.Model)
	if primary != "" {
		ctx = WithModel(ctx, primary)
	}
	if len(p.Fallbacks) > 0 {
		chain := []string{primary}
		if primary == "" {
			chain = nil
		}
		for _, m := range p.Fallbacks {
			if m = strings.TrimSpace(m); m != "" && m != primary {
				chain = append(chain, m)
			}
		}
		ctx = WithModelChain(ctx, chain)
	}
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	ctx = memory.WithScope(ctx, scope)
	// The agent's working directory (M792): file/shell tools operate inside
	// this workspace subdirectory. Escape-proofed by the setter.
	ctx = agent.WithWorkdir(ctx, p.Workdir)
	// And its identity + daily ceiling for the Governor's ledger (M793).
	return WithAgentIdent(ctx, p.Slug, p.MaxDailyMc)
}

// WithAgentIdent stamps the run with a named agent's identity and per-day
// spend ceiling (M793): every completion of the run is metered against the
// Governor's per-agent daily ledger and refused past the ceiling.
func WithAgentIdent(ctx context.Context, slug string, dailyMc int64) context.Context {
	if strings.TrimSpace(slug) == "" {
		return ctx
	}
	// Also stamp the agent slug under the kernel/agent key so provenance-aware
	// tools (memory, M851) can read who is acting via agent.AgentFromContext —
	// the runtime key here is private and additionally carries the daily ceiling.
	ctx = agent.WithAgent(ctx, slug)
	return context.WithValue(ctx, ctxKeyAgentIdent, agentIdent{slug: slug, dailyMc: dailyMc})
}

func agentIdentFromCtx(ctx context.Context) (string, int64) {
	if v, ok := ctx.Value(ctxKeyAgentIdent).(agentIdent); ok {
		return v.slug, v.dailyMc
	}
	return "", 0
}

func agentSlugFromCtx(ctx context.Context) string { s, _ := agentIdentFromCtx(ctx); return s }

func agentDailyMcFromCtx(ctx context.Context) int64 { _, d := agentIdentFromCtx(ctx); return d }

// WithModelChain sets the run's per-agent ordered model fallback chain (M787):
// the Governor tries these models in order, overriding the task type's
// configured chain. Carries a named agent's own fallbacks (roster M783).
func WithModelChain(ctx context.Context, chain []string) context.Context {
	if len(chain) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyModelChain, chain)
}

func modelChainFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxKeyModelChain).([]string); ok {
		return v
	}
	return nil
}

// WithTools restricts the run started with this context to the named tools only
// (a per-run allowlist). A non-nil slice — including an EMPTY one (no tools at all,
// for a pure-reasoning / safe one-off run) — activates the restriction; passing it
// is the only way to override, so an unrestricted run is simply one where this is
// never called. Names not registered are ignored.
func WithTools(ctx context.Context, allow []string) context.Context {
	return context.WithValue(ctx, ctxKeyTools, allow)
}

// toolsFromCtx returns the per-run tool allowlist and whether one was set. ok=false
// means "no restriction" (use all tools); ok=true with an empty/nil slice means
// "no tools".
func toolsFromCtx(ctx context.Context) ([]string, bool) {
	v, ok := ctx.Value(ctxKeyTools).([]string)
	return v, ok
}

// filterTools returns the subset of tools whose names are in allow (a registered
// name not present is dropped; an allow name with no matching tool is ignored).
// An empty/nil allow yields an empty map — no tools.
func filterTools(tools map[string]agent.Tool, allow []string) map[string]agent.Tool {
	keep := make(map[string]struct{}, len(allow))
	for _, n := range allow {
		keep[n] = struct{}{}
	}
	out := make(map[string]agent.Tool, len(keep))
	for name, tool := range tools {
		if _, ok := keep[name]; ok {
			out[name] = tool
		}
	}
	return out
}

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

// CancelRun cancels a single in-flight run by correlation id, leaving the
// kernel un-halted and every other run untouched (M32). This is the
// targeted counterpart to Halt's blunt "cancel everything and block new
// runs": an operator can kill one stuck run without pausing the whole
// daemon. Returns true if a matching live run was found and cancelled,
// false if there is no such active run (already finished, never existed,
// or wrong id).
//
// The cancel is the run context's own CancelFunc, so it cancels with
// context.Canceled — the agent loop's M30 terminal emitter then records
// task.failed(reason=canceled), distinct from a wall-clock timeout
// (DeadlineExceeded → reason=timeout, M31). We delete the entry here too;
// RunWith's defer also deletes it, but delete is idempotent so the race is
// harmless.
func (k *Kernel) CancelRun(corr string) bool {
	k.mu.Lock()
	cancel, ok := k.runs[corr]
	if ok {
		delete(k.runs, corr)
	}
	k.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
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

// assureVerifyMaxTokens bounds the verifier completion — it only emits a tiny
// JSON verdict, so a small cap keeps the completion check cheap.
const assureVerifyMaxTokens = 400

// RunAssured is the "do-it-for-sure" loop (M651): it runs the intent, asks a
// verifier whether the task was actually accomplished, and retries with the gap
// fed back — up to maxAttempts, stopping the moment the task is judged complete.
// Every attempt reuses corr (they run sequentially and never overlap), so the
// whole objective streams and journals under one correlation id. Returns the
// final answer and the loop result (attempts, completion, per-attempt history).
func (k *Kernel) RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assure.Result, error) {
	res, err := assure.Until(ctx, intent, maxAttempts,
		func(ctx context.Context, _ int, task string) (string, error) {
			return k.RunWith(ctx, corr, task)
		},
		func(ctx context.Context, task, answer string) (assure.Verdict, error) {
			return k.verifyCompletion(ctx, corr, task, answer)
		},
	)
	return res.Answer, res, err
}

// verifyCompletion asks the provider whether answer fully accomplishes task,
// parsing a strict-JSON verdict and journaling it under corr so `agt why` shows
// why an assured run retried or stopped. An unparseable verdict is treated as
// "not complete" (the bounded loop tries again rather than declaring a false
// success).
func (k *Kernel) verifyCompletion(ctx context.Context, corr, task, answer string) (assure.Verdict, error) {
	prompt := "You are a strict completion checker. Given a TASK and the ANSWER an agent produced, decide whether the answer FULLY accomplishes the task with nothing important left undone. Be skeptical: a plan or a promise to do it is NOT completion.\n\n" +
		"Reply with ONLY a JSON object and no other text: {\"complete\": true|false, \"gap\": \"<concise description of what is still missing; empty string if complete>\"}.\n\n" +
		"TASK:\n" + task + "\n\nANSWER:\n" + answer
	resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
		Model:         k.Model(),
		CorrelationID: corr,
		TaskType:      "verify",
		MaxTokens:     assureVerifyMaxTokens,
		Messages:      []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	})
	if err != nil {
		return assure.Verdict{}, err
	}
	v, ok := assure.ParseVerdict(resp.Message.Content)
	if !ok {
		v = assure.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "agent.agent-" + corr + ".assure",
		Kind:          event.KindAssureVerdict,
		Actor:         "assure",
		CorrelationID: corr,
		Payload:       map[string]any{"complete": v.Complete, "gap": v.Gap},
	})
	return v, nil
}

// visionDescribeMaxTokens bounds the sidecar caption — a description, not an essay.
const visionDescribeMaxTokens = 1024

// ErrNoVisionModel is returned by DescribeImages when no vision-capable model is
// available (the sidecar is disabled or no keyed provider has one).
var ErrNoVisionModel = errors.New("runtime: no vision-capable model available")

// DescribeImages runs the vision SIDECAR (M821): it sends the images to a keyed
// vision-capable model and returns a text description, so a run whose active
// model can't see images can still "read" them (the caller injects the returned
// text into the run). One-shot governor completion — no agent loop — routed to
// the vision model via per-request model routing. hint, if non-empty, replaces
// the default instruction. Returns ErrNoVisionModel when none is configured.
func (k *Kernel) DescribeImages(ctx context.Context, corr string, images []string, hint string) (string, error) {
	if len(images) == 0 {
		return "", nil
	}
	if k.cfg.VisionModel == nil {
		return "", ErrNoVisionModel
	}
	model, ok := k.cfg.VisionModel()
	if !ok || model == "" {
		return "", ErrNoVisionModel
	}
	prompt := hint
	if strings.TrimSpace(prompt) == "" {
		prompt = "Describe the attached image(s) in detail and transcribe any visible text. Be thorough and factual."
	}
	resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
		Model:         model,
		CorrelationID: corr,
		TaskType:      "vision",
		MaxTokens:     visionDescribeMaxTokens,
		Messages:      []agent.Message{{Role: agent.RoleUser, Content: prompt, Images: images}},
	})
	if err != nil {
		return "", err
	}
	// Journal the sidecar so `agt why` shows the active model was supplemented by
	// a vision model (reuse capability.rerouted; capability="vision").
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "agent.agent-" + corr + ".vision",
		Kind:          event.KindCapabilityRerouted,
		Actor:         "vision",
		CorrelationID: corr,
		Payload: map[string]any{
			"from_model": k.Model(),
			"to_model":   model,
			"capability": "vision",
			"images":     len(images),
		},
	})
	return resp.Message.Content, nil
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
	// Reject a correlation that is already running: two concurrent RunWith calls
	// sharing one id would clobber the run registry — the second's cancel overwrites
	// the first's k.runs[corr], and the first's deferred delete then removes the
	// second's entry, leaving a run uncancellable by Halt/CancelRun. The contract is
	// one id per run; enforce it instead of silently corrupting the registry. (M480)
	if _, running := k.runs[corr]; running {
		k.mu.Unlock()
		return "", fmt.Errorf("runtime: correlation %q is already running", corr)
	}
	// Per-run wall-clock budget (M31): when configured, the run context
	// carries a deadline so a slow provider / blocking tool can't hang a
	// run forever within a live session. The deadline cancels with
	// DeadlineExceeded (→ task.failed reason=timeout, M30), whereas the
	// cancel stored in k.runs (invoked by Halt) cancels with Canceled
	// (→ reason=canceled) — the two stay distinguishable. 0 = no cap.
	// A per-run override (WithRunTimeout, e.g. `agt run --timeout`) takes
	// precedence over the daemon-wide MaxDuration; either yields a deadline that
	// cancels with DeadlineExceeded.
	// Stamp the tenant identity onto the run context so tenant-aware tools can read it
	// (M219). No-op for the primary kernel (empty TenantID). Done before deriving runCtx
	// so the value propagates through the timeout/cancel context to every tool call.
	ctx = tenantctx.WithTenant(ctx, k.cfg.TenantID)

	maxDur := k.cfg.MaxDuration
	if d := runTimeoutFromCtx(ctx); d > 0 {
		maxDur = d
	}
	var runCtx context.Context
	var cancel context.CancelFunc
	if maxDur > 0 {
		runCtx, cancel = context.WithTimeout(ctx, maxDur)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	k.runs[corr] = cancel
	// Drain accounting (M883): Close waits (bounded) for in-flight runs to
	// settle before tearing down the stores they write to. Add under the same
	// lock as the halted check above, so no run can slip in after Halt flips
	// the flag and Close begins waiting.
	k.runWG.Add(1)
	// Live-steering control surface (M608): registered for the run's whole
	// lifetime so an operator can pause/step/inject from another goroutine. Wired
	// into the agent loop via LoopConfig.Steer below.
	rc := newRunControl()
	k.steers[corr] = rc
	k.mu.Unlock()

	defer k.runWG.Done()
	defer func() {
		k.mu.Lock()
		delete(k.runs, corr)
		delete(k.fanout, corr) // release this run's fan-out tally (M46)
		delete(k.tree, corr)   // release this tree's total sub-agent tally (M629)
		delete(k.steers, corr) // release the steering control (M608)
		// Cancel any still-pending async delegations of this tree (M881): an
		// un-awaited child must not outlive the run that spawned it. The spawn
		// goroutine observes the cancel, finishes, and journals its terminal
		// events; the handle is dropped here so the id is no longer awaitable.
		var orphans []context.CancelFunc
		for id, h := range k.spawns {
			if h.rootCorr == corr || h.parentCorr == corr {
				orphans = append(orphans, h.cancel)
				delete(k.spawns, id)
			}
		}
		k.mu.Unlock()
		for _, c := range orphans {
			c()
		}
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
	// So warden-backed tools (shell) stamp this run's correlation onto their
	// warden.executed events — making the isolation profile show up in the run's
	// timeline and walkable by `agt why`.
	runCtx = warden.WithCorrelation(runCtx, corr)

	// Memory injection: recall relevant records and prepend them to the
	// system prompt so the model starts the task already knowing what
	// Agezt remembers. The recall is journaled (memory.retrieved) under
	// corr, so `agt why` shows exactly what knowledge was surfaced.
	// Per-run system-prompt override (WithSystem): a one-off persona/instruction
	// set for this run only; falls back to the kernel's configured System. Memory /
	// world / skill injection below still layer on top.
	system := k.System() // live persona (M710), editable at runtime
	if s := systemFromCtx(runCtx); s != "" {
		system = s
	}
	if k.cfg.MemoryInject {
		topK := k.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		// Scoped to the run's agent identity (M786): a named agent's private
		// notes surface in its injected context; an unscoped run sees shared
		// memory only (RecallScoped with "" ≡ the previous Recall behaviour).
		if hits, err := k.memory.RecallScoped(corr, intent, topK, memory.ScopeFrom(runCtx)); err == nil && len(hits) > 0 {
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
	var activatedSkillIDs []string
	if k.cfg.SkillInject {
		topK := k.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		if hits, err := k.forge.Activate(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectSkills(system, hits)
			for _, h := range hits {
				activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
			}
		}
	}

	// Per-run model override (WithModel) — used by the OpenAI-compatible API to
	// honour the request's `model`. Falls back to the live default model
	// (k.Model(), hot-swappable via SetModel on provider reload — M816).
	model := k.Model()
	if m := modelFromCtx(runCtx); m != "" {
		model = m
	}

	// Per-run tool restriction (WithTools): an allowlist (possibly empty = no
	// tools) scopes what this run may call, without changing the kernel's tool
	// set. Forged script tools (M794) and live MCP attachments (M796) are
	// merged BEFORE the filter so a restricted run only sees the dynamic
	// tools its allowlist grants.
	runTools := k.mergeMCPTools(k.mergeScriptTools(k.tools))
	if allow, ok := toolsFromCtx(runCtx); ok {
		runTools = filterTools(runTools, allow)
	}

	// Host-environment preamble (M609): prepend OS/arch, the shell the shell tool
	// uses, the shared workspace dir, the date, and THIS run's tools — so the
	// model acts correctly on this host instead of guessing. Injected last (after
	// memory/world/skills and after runTools is resolved) so it sits at the top of
	// the system prompt and reflects any per-run tool restriction.
	if k.cfg.EnvironmentInject {
		system = injectEnvironment(system, k.cfg.WorkspaceRoot, runTools, time.Now())
	}

	// Context budget (SPEC-10 §3): an explicit budget wins; otherwise, in auto
	// mode, derive one from the resolved model's catalog context window. An
	// unknown model leaves compaction off (0).
	ctxBudget := k.cfg.ContextBudget
	if ctxBudget == 0 && k.cfg.ContextBudgetAuto {
		// Read the catalog through the locked accessor: ReloadCatalog swaps the
		// k.catalog field under k.mu, and this hot path runs concurrently with an
		// operator's `catalog sync`/`provider reload`, so a direct field read here is
		// a data race (no happens-before, possible stale/torn read). (M477)
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(model); m != nil {
				ctxBudget = agent.AutoContextBudgetChars(m.Limit.Context)
			}
		}
	}

	// Abstractive summary of elided tool outputs (M398): opt-in, and only worth
	// wiring when compaction is actually active for this run.
	var summarizeElided func(context.Context, string) (string, error)
	if k.cfg.ContextSummarize && (ctxBudget > 0 || k.cfg.ContextBudgetAuto) {
		summarizeElided = makeElidedSummarizer(k.cfg.Provider, model, corr)
	}

	answer, err := agent.Run(runCtx, agent.LoopConfig{
		Provider:             k.cfg.Provider,
		Tools:                runTools,
		Bus:                  k.bus,
		Model:                model,
		TaskType:             "chat",                    // M703: main agent loop → "chat" routing target
		ModelChain:           modelChainFromCtx(runCtx), // M787: a named agent's own fallbacks
		Agent:                agentSlugFromCtx(runCtx),
		AgentDailyCeilingMc:  agentDailyMcFromCtx(runCtx),
		System:               system,
		MaxIter:              k.cfg.MaxIter,
		MaxAutoContinue:      k.cfg.MaxAutoContinue,  // M833: autonomous continue past MaxIter
		AutoContinueWait:     k.cfg.AutoContinueWait, // M833
		ToolTimeout:          k.cfg.ToolTimeout,
		MaxParallelTools:     k.cfg.MaxParallelTools, // M880: in-turn parallel tool dispatch
		Actor:                actor,
		CorrelationID:        corr,
		Policy:               k.policyHook,
		Images:               imagesFromCtx(runCtx),   // M93: image attachments (vision-gated upstream)
		JSONMode:             jsonModeFromCtx(runCtx), // M314: structured-output request
		MaxRunCostMicrocents: maxCostFromCtx(runCtx),  // M166: per-run cost cap
		CostFn:               governor.CostMicrocents,
		Artifacts:            k.artifacts, // M390: offload oversized tool outputs (SPEC-04 §3.6)
		ArtifactThreshold:    k.cfg.ArtifactThreshold,
		ContextBudget:        ctxBudget,                 // M393/M394: context budgeting (SPEC-10 §3)
		ContextProtectFirst:  k.cfg.ContextProtectFirst, // M395: shield the earliest grounding
		SummarizeElided:      summarizeElided,           // M398: abstractive summary of dropped outputs
		Steer:                rc,                        // M608: live operator steering
	}, intent)

	// Deregister the steering control the instant the agent loop returns — BEFORE
	// the post-run work below (skill-outcome attribution, memory distillation,
	// which itself makes an LLM call). The outer defer also deletes it, but that
	// runs only after all post-processing; without this an operator pausing/
	// steering in that window would get a false success against a loop that has
	// already finished and will never Drain again (M608). delete is idempotent.
	k.mu.Lock()
	delete(k.steers, corr)
	k.mu.Unlock()

	// Attribute the run's outcome to the skills it activated, so an active skill
	// that repeatedly fails in production is auto-quarantined (SPEC-05 §5). This
	// is the production caller of RecordOutcome; best-effort bookkeeping that never
	// changes the run result.
	if k.forge != nil && len(activatedSkillIDs) > 0 {
		k.forge.RecordOutcome(corr, activatedSkillIDs, err == nil)
	}

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
	// Shadow-evaluate relevant shadow skills against this completed run (SPEC-05
	// §5.2). We're past the err!=nil early return, so the run succeeded — a failed
	// run is a poor yardstick for "would it have helped".
	if k.cfg.ShadowEval && k.forge != nil {
		k.maybeShadowEval(runCtx, corr, intent, answer)
	}
	return answer, nil
}

// shadowEvalLimit bounds how many shadow candidates are judged per run, so the
// extra (opt-in) provider calls stay bounded regardless of how many shadow
// skills match the intent.
const shadowEvalLimit = 2

// maybeShadowEval judges the shadow skills relevant to a just-completed run
// (SPEC-05 §5.2). Best-effort: a judge failure is journaled but never affects the
// run, which has already returned its answer.
func (k *Kernel) maybeShadowEval(ctx context.Context, corr, intent, answer string) {
	if err := k.forge.ShadowEvaluate(ctx, corr, k.cfg.Provider, k.Model(), intent, answer, shadowEvalLimit); err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject:       "skill.shadow_eval_failed",
			Kind:          event.KindSkillShadowEval,
			Actor:         "forge",
			CorrelationID: corr,
			Payload:       map[string]any{"error": err.Error()},
		})
	}
}

// elidedSummaryMaxTokens bounds the abstractive summary call (M398): one short
// line, so a small cap keeps the extra spend negligible and the latency low.
const elidedSummaryMaxTokens = 64

// elidedSummaryInputCap bounds how much of a dropped output is fed to the
// summarizer — enough to summarise, while keeping the summary call's own input
// (and therefore its cost) bounded regardless of how large the output was.
const elidedSummaryInputCap = 8 << 10

// makeElidedSummarizer builds the LoopConfig.SummarizeElided closure: a bounded,
// single-shot provider call that condenses a dropped tool output to one line
// (M398). It routes through the same provider (the Governor) as the run, so the
// extra call is billed and attributed to the run via corr. Errors propagate; the
// loop swallows them and falls back to the deterministic head snippet.
func makeElidedSummarizer(provider agent.Provider, model, corr string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, output string) (string, error) {
		in := output
		if len(in) > elidedSummaryInputCap {
			in = in[:elidedSummaryInputCap]
		}
		resp, err := provider.Complete(ctx, agent.CompletionRequest{
			Model:         model,
			CorrelationID: corr,
			TaskType:      "summarize",
			MaxTokens:     elidedSummaryMaxTokens,
			Messages: []agent.Message{{
				Role:    agent.RoleUser,
				Content: "Summarize this tool output in one short line for an agent's working memory. Output only the summary.\n\n" + in,
			}},
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
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
		// A bundled skill (agentskills.io shape, M847) ships reference files and
		// scripts. List them and tell the agent how to reach them: read a reference
		// with `skill op=read`, run a script with shell/code_exec from the dir that
		// `skill op=files` reports. This is what lets a skill say "run scripts/setup.sh
		// to install the CLI" and have the agent actually do it.
		if len(s.Resources) > 0 {
			fmt.Fprintf(&b, "Bundled resources (use the `skill` tool: op=files for the directory, op=read \"<path>\" to read one; run scripts with shell/code_exec):\n")
			for _, r := range s.Resources {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
		}
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// shellHinter is the optional interface a shell-like tool implements to tell the
// environment preamble the EXACT interpreter it runs (binary + flag), so the
// guidance reflects an operator's shell override rather than a GOOS guess.
type shellHinter interface{ ShellHint() (string, string) }

// injectEnvironment prepends a concise host-environment preamble to the system
// prompt (M609): OS/arch, the shell the shell tool uses (with command-style
// guidance so the model doesn't try `ls` on Windows), the shared workspace dir,
// the date, and the run's available tools. This is the single highest-leverage
// fix for blind trial-and-error tool use on non-Unix hosts. `now` is passed in
// for deterministic tests.
func injectEnvironment(system, workspaceRoot string, tools map[string]agent.Tool, now time.Time) string {
	var b strings.Builder
	b.WriteString("## Runtime environment\n")
	b.WriteString("You run on a real host — act for THIS environment, do not assume Unix.\n")
	fmt.Fprintf(&b, "- OS / arch: %s / %s\n", stdruntime.GOOS, stdruntime.GOARCH)

	// Shell line: prefer the shell tool's own hint (honours overrides); fall back
	// to the GOOS default the shell tool would pick.
	shellBin, shellArg := defaultShellHint()
	if t, ok := tools["shell"]; ok {
		if h, ok := t.(shellHinter); ok {
			shellBin, shellArg = h.ShellHint()
		}
	}
	fmt.Fprintf(&b, "- Shell tool runs commands via `%s %s`. %s\n", shellBin, shellArg, shellGuidance(shellBin))

	if workspaceRoot != "" {
		fmt.Fprintf(&b, "- Working directory (shell + file tools both operate here): %s\n", workspaceRoot)
	}
	fmt.Fprintf(&b, "- Today: %s\n", now.Format("2006-01-02"))

	if len(tools) > 0 {
		names := make([]string, 0, len(tools))
		for name := range tools {
			names = append(names, name)
		}
		slices.Sort(names)
		b.WriteString("- Tools available this run:\n")
		for _, name := range names {
			fmt.Fprintf(&b, "  - %s — %s\n", name, firstSentence(tools[name].Definition().Description))
		}
	}
	if brief := capabilityBriefing(tools); brief != "" {
		b.WriteString(brief)
	}
	b.WriteString("Some capabilities require operator approval and may be denied; if a call is denied, adapt your approach rather than repeating it.\n")

	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// capabilityBriefing tells the agent, plainly, how much it is allowed to do —
// so it knows its own reach instead of timidly assuming limits (M848). AGEZT is
// default-allow (owner law): every capability is on unless explicitly opted out,
// so an agent should marshal whatever it takes to finish the task — write and run
// code, install and run CLIs and packages, build whole apps and services. The
// briefing is tuned to the tools ACTUALLY present this run (no point promising
// code execution if code_exec was opted out), and stays honest about the few real
// rails: explicit operator denials, budgets, and the SSRF/secret guards. Returns
// "" when none of the relevant tools are available.
func capabilityBriefing(tools map[string]agent.Tool) string {
	_, hasShell := tools["shell"]
	_, hasCode := tools["code_exec"]
	_, hasFile := tools["file"]
	_, hasForge := tools["tool_forge"]
	_, hasSkill := tools["skill"]
	if !hasShell && !hasCode && !hasFile {
		return "" // nothing to brief — this run can't build or run anything
	}
	var b strings.Builder
	b.WriteString("## What you can do — act without artificial limits\n")
	b.WriteString("You are a full agent on a real machine, not a sandboxed chat. To accomplish the task you may do whatever it takes — there are no artificial capability limits. Be bold: build it, install it, run it.\n")
	if hasCode {
		b.WriteString("- Write and run real code — Python, Node/JavaScript, Deno — via code_exec. Build scripts, prototypes, data pipelines, whole programs.\n")
	}
	if hasShell {
		b.WriteString("- Install and run anything the host supports via the shell: CLI tools, npm / pip / cargo / go packages, build systems, even long-running background services. If a command is missing, install it, then use it.\n")
	}
	if hasFile {
		b.WriteString("- Create and edit as many files, projects, and applications as the task needs in your working directory.\n")
	}
	if hasForge {
		b.WriteString("- When a one-off script isn't enough, forge your own durable tool (tool_forge) so the capability persists.\n")
	}
	if hasSkill {
		b.WriteString("- Capture what works as a reusable skill — including bundled reference files and scripts — so future runs reuse it (skill op=learn / op=files / op=read).\n")
	}
	b.WriteString("Default to action: prefer doing the work over asking whether you're allowed. The only real limits are explicit — a denied approval, a spend budget, and the network/secret guards (no SSRF, secrets stay redacted). Everything else is yours to use.\n")
	return b.String()
}

// defaultShellHint mirrors the shell tool's platform default for callers that
// can't reach the live tool (e.g. tools map without a shell). cmd on Windows,
// sh elsewhere — kept in sync with plugins/tools/shell.resolveShell.
func defaultShellHint() (string, string) {
	if stdruntime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// shellGuidance returns one line of command-style advice keyed off the shell
// binary, so the model uses native commands (the #1 source of wasted iterations
// on Windows was the model reflexively trying `ls`/`cat`/`rm`).
func shellGuidance(shellBin string) string {
	switch strings.ToLower(filepath.Base(shellBin)) {
	case "cmd", "cmd.exe":
		return "Use native Windows commands (dir, type, copy, del, move, findstr) — NOT ls/cat/rm/cp/mv/grep. Chain with `&&`."
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return "Use PowerShell cmdlets (Get-ChildItem, Get-Content, Copy-Item, Remove-Item) or their aliases."
	default:
		return "Use standard POSIX commands (ls, cat, grep, rm). Chain with `&&`."
	}
}

// firstSentence trims a tool description to its first sentence (or first line),
// keeping the environment preamble compact when a tool has a long description.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	return strings.TrimSpace(s)
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
	if _, err := k.forge.Propose(ctx, corr, k.cfg.Provider, k.Model(), intent, transcript); err != nil {
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
	if _, err := k.memory.Distill(ctx, corr, k.cfg.Provider, k.Model(), intent, transcript); err != nil {
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

// Causes returns the causation ancestry of an event: the chain of events linked
// by causation_id from the root cause down to (and including) the target,
// ordered oldest-first (root → … → target). This is the provenance walk
// SPEC-01 §7.1 describes ("agt why walks the chain backwards"), and it is
// distinct from Why, which groups by correlation_id.
//
// Causation crosses correlation boundaries where correlation cannot: a Pulse
// delta/salience/initiative event carries its OWN per-chain correlation but
// links to the originating tick (a different correlation) only via causation_id
// — so the tick is reachable here yet invisible to Why. Likewise a channel
// reply links to the inbound message that caused it.
//
// The walk is cycle-guarded (a corrupt or forged journal could contain a
// causation loop; the runtime never emits one) and stops at the root
// (causation_id == "") or a dangling link (the referenced parent is absent).
// Returns the target alone when it has no causation parent.
func (k *Kernel) Causes(eventID string) ([]*event.Event, error) {
	byID := make(map[string]*event.Event)
	var target *event.Event
	err := k.journal.Range(func(e *event.Event) error {
		byID[e.ID] = e
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
	// Walk causation_id backwards from the target, newest-first.
	chain := make([]*event.Event, 0, 8)
	seen := make(map[string]struct{})
	for cur := target; cur != nil; {
		if _, dup := seen[cur.ID]; dup {
			break // cycle guard: a forged causation loop must terminate
		}
		seen[cur.ID] = struct{}{}
		chain = append(chain, cur)
		if cur.CausationID == "" {
			break // reached the root cause
		}
		cur = byID[cur.CausationID] // nil if the parent is missing → walk ends
	}
	// Reverse in place to oldest-first (root → … → target).
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// ParentOf returns the lead run's correlation for a sub-agent run, or "" if
// childCorr was not spawned via delegation (M42). It scans the journal for a
// subagent.spawned event whose payload names childCorr as its child — the
// spawn lives under the PARENT correlation, so this is the only way to walk
// child→parent (the parent→child direction is already visible because the
// spawn is in the parent's own chain). A single forward scan; the last match
// wins (a child has exactly one spawn in practice).
func (k *Kernel) ParentOf(childCorr string) string {
	if childCorr == "" {
		return ""
	}
	parent := ""
	_ = k.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindSubAgentSpawned {
			return nil
		}
		var p struct {
			Child  string `json:"child_correlation"`
			Parent string `json:"parent"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil
		}
		if p.Child == childCorr && p.Parent != "" {
			parent = p.Parent
		}
		return nil
	})
	return parent
}

// Verify replays every event and confirms the BLAKE3 chain is intact.
// Returns nil on success.
func (k *Kernel) Verify() error {
	return k.journal.Verify()
}
