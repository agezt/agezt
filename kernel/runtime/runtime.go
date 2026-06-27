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
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/market"
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

	"github.com/agezt/agezt/internal/apperrors"
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

	// MCPHTTPDialer handshakes one REMOTE MCP server over Streamable HTTP on
	// attach (M904, #39) — used when a registration carries a URL instead of a
	// command. Nil means the production dialer (mcp.DialHTTP); tests inject fakes.
	MCPHTTPDialer mcp.HTTPDialer

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
	// ProfileInject (M1000) prepends the learned operator profile (a separate
	// shared-memory namespace, synthesized by DistillProfile) to every non-system
	// run's System prompt, so the assistant knows who it works for. Gated by
	// AGEZT_USER_PROFILE (default on); a no-op until a profile exists.
	ProfileInject bool
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

	// MarketTool registers the in-process `market` tool so an agent can discover
	// and install capability packs (skills + MCP servers + tools) from the
	// marketplace mid-task. The tool resolves the kernel's market manager lazily
	// (the daemon wires it via SetMarket after Open); when no manager is wired the
	// tool reports the marketplace is unavailable. Off by default.
	MarketTool bool

	// Voice, when non-nil, registers the in-process `voice` tool so an agent can
	// transcribe inbound audio (speech-to-text) and synthesize spoken replies
	// (text-to-speech). The kernel never picks an implementation — the daemon
	// injects one (typically the OpenAI-compatible voice adapter plugin) built
	// from AGEZT_STT_* / AGEZT_TTS_*. Unset → no voice tool.
	Voice Voice

	// ImageGenerator, when non-nil, registers the in-process `image_generate`
	// tool so an agent can generate images from a prompt (M997). The daemon
	// injects one (the OpenAI-compatible image plugin) built from AGEZT_IMAGE_*.
	// Unset → no image tool. Generated images are saved as artifacts.
	ImageGenerator ImageGen
	// Reranker, when non-nil, registers the in-process `rerank` tool so an agent
	// can reorder candidate documents by relevance with a dedicated reranking
	// model (M997). The daemon injects one (the Cohere/Jina-style rerank plugin)
	// built from AGEZT_RERANK_*. Unset → no rerank tool.
	Reranker Reranker

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

	// ToolDiscoveryMax, when > 0, enables deterministic lexical tool discovery:
	// each provider call is offered at most this many relevant tools instead of
	// every registered schema. This is the CH-03 bridge to semantic discovery;
	// future embedding-backed selectors can replace the scorer without changing
	// the agent loop contract. 0 preserves the historical "offer all" behaviour.
	ToolDiscoveryMax int

	// ObservationDeltas, when true, makes repeated observations of the same
	// tool/input pair return a structured delta to the model while retaining the
	// full raw output in the journal. Off by default for compatibility. (CH-04)
	ObservationDeltas bool

	// EpistemicEscalation, when true, lets the runtime's external calibration
	// gate route otherwise-allowed tool calls to HITL approval when journaled
	// failure conditions, low effect confidence, temporal sensitivity, or novel
	// dynamic tool surfaces make the model's proposal unsafe to execute directly.
	// Off by default for compatibility; policy.decision still journals the
	// epistemic signals either way.
	EpistemicEscalation bool

	// IntentRegretGating, when true, routes otherwise-allowed tool calls to HITL
	// approval when the user utterance is underdetermined and the proposed action
	// has high wrong-action regret. Off by default for compatibility; intent
	// interpretation is still journaled either way.
	IntentRegretGating bool

	// PromptInjectionGuard selects how the daemon handles an otherwise-allowed
	// effectful tool call that is downstream (within the causal window) of
	// untrusted external content containing directive-like text:
	//   PromptInjectionOn   (default) — route it to HITL approval.
	//   PromptInjectionWarn — allow it, but journal a prompt_injection.warned
	//                         event so the chat can surface a passive banner.
	//   PromptInjectionOff  — no active intervention.
	// The observation boundary, untrusted rendering, and audit metadata are
	// always on regardless. A chat run can downgrade On→warn for itself via the
	// trusted-observations flag (WithTrustedObservations).
	PromptInjectionGuard PromptInjectionMode

	// DisableHeuristicBypass turns off deterministic fast paths for known-safe
	// intents such as current time/date queries. The default keeps the narrow
	// CH-09 bypass layer enabled so trivial solved subproblems do not spend LLM
	// tokens.
	DisableHeuristicBypass bool

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

	memory       *memory.Manager
	memoryDir    *memory.FileStore
	world        *worldmodel.Graph
	worldDir     *worldmodel.FileStore
	forge        *skill.Forge
	skillDir     *skill.FileStore
	marketMgr    *market.Manager
	standing     *standing.Store
	roster       *roster.Store
	toolForge    *toolforge.Store
	mcpStore     *mcp.Store
	workflows    *workflow.Store
	artifacts    *artifact.Store
	artIndex     *artifact.Index // metadata sidecar over artifacts (M822): browsable/deletable entries
	lake         *datalake.Lake  // Personal Data Lake (M834): agent-built structured collections
	reflect      *reflect.Engine
	schedules    *cadence.Store        // persistent typed schedule store (autonomy)
	schedEngine  *cadence.Engine       // live cadence resident, set by the daemon after Open
	agentGW      *agentgw.Gateway      // agent subprocess gateway (agent SDK)
	configCenter *configcenter.Center  // config center for agent SDK config access
	tools        map[string]agent.Tool // cfg.Tools + the memory/world tools (when enabled)

	// conductorExec is the optional code-execution backend the Conductor's
	// Verifier role uses to actually RUN a worker's code (M997). Injected once
	// after Open by the daemon (SetConductorExec, wired from the code_exec tool)
	// so the kernel never imports the codeexec plugin. nil when the sandbox is
	// off — the Verifier then falls back to LLM critique.
	conductorExec CodeExecutor

	catalogStore *catalog.Store
	catalog      *catalog.Catalog // snapshot — refreshable via ReloadCatalog

	// Fine-grained mutexes to reduce lock contention. Lock ordering to prevent
	// deadlocks (always acquire in this order):
	//   configMu (light config) < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu
	configMu sync.Mutex // guards: system, model, cfg, catalog, schedEngine
	runsMu   sync.Mutex // guards: halted, runs
	fanoutMu sync.Mutex
	treeMu   sync.Mutex
	steersMu sync.Mutex
	spawnsMu sync.Mutex
	mcpMu    sync.Mutex // guards: mcpConns

	halted bool
	system string                        // live daemon default identity / system prompt (M710); seeded from cfg.System, editable at runtime
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
		return nil, apperrors.WrapSimple("runtime: journal", err)
	}
	st, err := state.Open(filepath.Join(cfg.BaseDir, "state"))
	if err != nil {
		j.Close()
		return nil, apperrors.WrapSimple("runtime: state", err)
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
	sched := scheduler.New(scheduler.Config{Bus: kbus, Monitor: scheduler.ContextInvariantMonitor})

	catDir := cfg.CatalogDir
	if catDir == "" {
		catDir = filepath.Join(cfg.BaseDir, "catalog")
	}
	mstore, err := memory.Open(filepath.Join(cfg.BaseDir, "memory"))
	if err != nil {
		j.Close()
		st.Close()
		return nil, apperrors.WrapSimple("runtime: memory", err)
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
		return nil, apperrors.WrapSimple("runtime: worldmodel", err)
	}
	wgraph := worldmodel.NewGraph(wstore, kbus)

	skstore, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		return nil, apperrors.WrapSimple("runtime: skills", err)
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
		return nil, apperrors.WrapSimple("runtime: cadence", err)
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
		return nil, apperrors.WrapSimple("runtime: artifacts", err)
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
		return nil, apperrors.WrapSimple("runtime: artifact index", err)
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
		return nil, apperrors.WrapSimple("runtime: data lake", err)
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
		return nil, apperrors.WrapSimple("runtime: standing", err)
	}

	rstore, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, apperrors.WrapSimple("runtime: roster", err)
	}

	tfstore, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, apperrors.WrapSimple("runtime: toolforge", err)
	}

	mcpstore, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, apperrors.WrapSimple("runtime: mcp", err)
	}

	wfstore, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		wstore.Close()
		skstore.Close()
		return nil, apperrors.WrapSimple("runtime: workflows", err)
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
	// The market tool needs k.Market (wired by the daemon after Open); register
	// it now and bind its lazy getter just after k is built (same map k holds).
	var mktTool *marketTool
	if cfg.MarketTool {
		mktTool = newMarketTool()
		effTools["market"] = mktTool
	}
	// The voice tool needs the kernel's artifact store (bound just after k is
	// built) to persist synthesized audio.
	var vTool *voiceTool
	if cfg.Voice != nil {
		vTool = newVoiceTool(cfg.Voice)
		effTools["voice"] = vTool
	}
	// image_generate needs the artifact store too (bound just after k is built).
	var imgTool *imageTool
	if cfg.ImageGenerator != nil {
		imgTool = newImageTool(cfg.ImageGenerator)
		effTools["image_generate"] = imgTool
	}
	if cfg.Reranker != nil {
		effTools["rerank"] = newRerankTool(cfg.Reranker)
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
			return nil, apperrors.WrapSimple("runtime: catalog load", err)
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
	if mktTool != nil {
		mktTool.manager = k.Market
	}
	if imgTool != nil {
		imgTool.saveArtifact = k.artifacts.Put
	}
	if vTool != nil {
		vTool.saveArtifact = k.artifacts.Put
	}

	// Config Center for agent SDK config access (M???)
	configCenter, err := configcenter.Open(configcenter.DefaultConfig(cfg.BaseDir))
	if err != nil {
		j.Close()
		st.Close()
		mstore.Close()
		return nil, apperrors.WrapSimple("runtime: configcenter", err)
	}
	// Wire approval registry for HITL support
	if apr != nil {
		configCenter.SetApprovalRegistry(apr)
	}
	k.configCenter = configCenter

	// Agent Gateway for subprocess communication (Agent SDK)
	gwCfg := agentgw.DefaultGatewayConfig(cfg.BaseDir)
	// Token signing key: a per-install secret persisted under the base dir (or
	// $AGEZT_AGENTGW_TOKEN_SECRET), shared with the `agt` CLI. Never the old
	// hardcoded "change-me-in-production" constant.
	secret, err := agentgw.ResolveTokenSecret(cfg.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve agentgw token secret: %w", err)
	}
	gwCfg.TokenSecret = secret
	// Override socket path from environment if set (useful for Windows TCP testing)
	if sockPath := os.Getenv("AGEZT_AGENTGW_SOCKET"); sockPath != "" {
		gwCfg.SocketPath = sockPath
	}
	agentGW := agentgw.NewGateway(gwCfg)
	agentGW.Attach(kbus, mgr, rstore)
	agentGW.SetConfigCenter(configCenter)
	agentGW.SetAuditJournal(j) // wire the audit trail (was a nil no-op)
	k.agentGW = agentGW

	// Start the gateway listener in background
	go func() {
		if err := agentGW.Listen(context.Background()); err != nil {
			slog.Error("runtime: agentgw listen", "error", err)
		}
	}()

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
		func() error { return k.agentGW.Close() },
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

// AgentGateway returns the Agent Gateway for subprocess communication.
// The gateway is initialized but not started during Open. Call
// AgentGateway().Listen(ctx) to start it.
func (k *Kernel) AgentGateway() *agentgw.Gateway { return k.agentGW }

// Schedules returns the persistent typed schedule store (autonomy). The cadence
// resident fires due agent/workflow/system-task/tool targets; `agt schedule`
// manages them.
func (k *Kernel) Schedules() *cadence.Store { return k.schedules }

// SetScheduleEngine records the live cadence resident so status/doctor/UI
// surfaces can observe whether scheduled work is currently running. It is set by
// the daemon, not Open, because cmd/agezt owns the schedule target dispatcher.
func (k *Kernel) SetScheduleEngine(e *cadence.Engine) {
	k.configMu.Lock()
	k.schedEngine = e
	k.configMu.Unlock()
}

// ScheduleEngine returns the live cadence resident when the daemon has started
// it. Nil means schedules can still be managed in the store but no resident is
// currently attached in this process.
func (k *Kernel) ScheduleEngine() *cadence.Engine {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.schedEngine
}

// World returns the world-model graph backing `agt world`, run-time entity
// injection, and the Pulse salience relevance signal. Always non-nil after
// Open.
func (k *Kernel) World() *worldmodel.Graph { return k.world }

// Forge returns the skill manager backing `agt skill`, run-time skill
// activation, and post-run skill proposal. Always non-nil after Open.
func (k *Kernel) Forge() *skill.Forge { return k.forge }

// Market returns the capability marketplace manager (skill/MCP/tool packs). It is
// nil until the daemon wires it via SetMarket (the built-in catalogue is a plugin
// the kernel must not import, so it is injected from cmd/agezt).
func (k *Kernel) Market() *market.Manager { return k.marketMgr }

// SetMarket injects the marketplace manager (from cmd/agezt, with the built-in
// Official library + this kernel's Forge/MCP as the install targets).
func (k *Kernel) SetMarket(m *market.Manager) { k.marketMgr = m }

// Artifacts returns the content-addressed artifact store (SPEC-04 §3.6), where
// the loop offloads oversized tool outputs. Used by retrieval surfaces.
func (k *Kernel) Artifacts() *artifact.Store { return k.artifacts }

// Voice returns the configured voice adapter (STT/TTS), or nil when unset. The
// channel inbound path uses it to auto-transcribe inbound voice notes.
func (k *Kernel) Voice() Voice { return k.cfg.Voice }

// ArtifactIndex returns the metadata index over the blob store (M822) — the
// browsable/deletable per-arrival entries (inbound images, tool outputs) the
// file manager and inbound-image persistence use.
func (k *Kernel) ArtifactIndex() *artifact.Index { return k.artIndex }

// DataLake returns the Personal Data Lake (M834) — the file-based structured
// collections agents build and share, surfaced by the `db` tool and the Web UI.
func (k *Kernel) DataLake() *datalake.Lake { return k.lake }

// Standing returns the standing wake-rule store (SPEC-16 §4), backing `agt
// standing`. Always non-nil after Open.
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
func (k *Kernel) SetProfileRetired(ref string, retired bool, reason ...string) (roster.Profile, error) {
	p, err := k.roster.SetRetired(ref, retired, reason...)
	if err != nil {
		return roster.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "reason": p.RetiredReason, "action": action},
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
	// Shipped guardians (System) are protected from hard delete (M961): they are
	// the daemon's own self-healing fleet. They can still be paused or retired.
	if p, ok := k.roster.Get(ref); ok && p.System {
		return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
	}
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
	k.runsMu.Lock()
	defer k.runsMu.Unlock()
	return len(k.runs)
}

// ActiveRunIDs returns the correlation ids of the runs in flight right now —
// the live keys of the cancel registry, sorted for determinism. This is the
// "what is running" the overseer (M850) cancels by id, distinct from the
// journal-derived run history (CmdRunsList): these are exactly the runs a
// CancelRun can still stop. Safe under concurrent run starts/completes.
func (k *Kernel) ActiveRunIDs() []string {
	k.runsMu.Lock()
	defer k.runsMu.Unlock()
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

// ConfigCenter returns the Config Center instance, or nil if not configured.
func (k *Kernel) ConfigCenter() *configcenter.Center { return k.configCenter }

// Model returns the live default model name. Empty when the daemon uses
// provider defaults rather than an override. Seeded from cfg.Model at Open and
// hot-swapped via SetModel when the provider is reloaded (M816), so it must be
// mu-guarded like the default identity. Used by `agt config show` and every run that
// builds a CompletionRequest without an explicit per-run/per-task model.
func (k *Kernel) Model() string {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.model
}

// SetModel replaces the live default model id. The next run picks it up — no
// restart. Paired with SetSystem-style persistence: the daemon's provider
// reload calls this after AGEZT_MODEL changes so a wizard/Config-Center edit
// takes effect in place instead of waiting for the next boot (M816).
func (k *Kernel) SetModel(m string) {
	k.configMu.Lock()
	k.model = m
	k.configMu.Unlock()
}

// SetCouncilMembers replaces the live default Council of Elders membership
// (M839). The next council convening picks it up — no restart. Paired with
// persistence: handleCouncilSet writes AGEZT_COUNCIL_MEMBERS to the settings
// store, then calls this so the kernel picks up the new membership immediately.
func (k *Kernel) SetCouncilMembers(members func() []CouncilMember) {
	k.configMu.Lock()
	k.cfg.CouncilMembers = members
	k.configMu.Unlock()
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

// System returns the live daemon default identity prompt. Empty when none is set.
// Seeded from cfg.System at Open and editable at runtime via SetSystem (M710).
// `agt config show` uses it only to report PRESENCE, not content (which could
// carry proprietary instructions); the dedicated default-identity surface returns
// the content for the owner to edit.
func (k *Kernel) System() string {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.system
}

// SetSystem replaces the live daemon default identity prompt. The next default
// run picks it up — no restart. Persistence (so it survives a restart) is the
// control plane's job: it writes AGEZT_SYSTEM_PROMPT to the config store
// alongside this.
func (k *Kernel) SetSystem(s string) {
	k.configMu.Lock()
	k.system = s
	k.configMu.Unlock()
}

// Catalog returns the currently-loaded provider/model catalog. The
// returned pointer is the live snapshot; callers should treat it as
// read-only and re-call after ReloadCatalog if they need fresh data.
func (k *Kernel) Catalog() *catalog.Catalog {
	k.configMu.Lock()
	defer k.configMu.Unlock()
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
		return cat, false, apperrors.WrapSimple("runtime: provider reload", err)
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
	k.configMu.Lock()
	k.catalog = cat
	k.configMu.Unlock()
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
	k.runsMu.Lock()
	if k.halted {
		k.runsMu.Unlock()
		return nil, ErrHalted
	}
	if planID == "" {
		planID = "plan-" + ulid.New()
	}
	runCtx, cancel := context.WithCancel(ctx)
	k.runs[planID] = cancel
	k.runsMu.Unlock()

	defer func() {
		k.runsMu.Lock()
		delete(k.runs, planID)
		k.runsMu.Unlock()
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
	def, _ := agent.PolicyToolDefFromContext(ctx)
	bundle := k.approvalDecisionBundle(tc.Name, out.Capability, tc.Input, def)
	verdict.EffectClass = bundle.EffectClass
	verdict.AffectedResources = append([]string(nil), bundle.AffectedResources...)
	ep := k.epistemicGate(tc.Name, out.Capability, tc.Input, def, bundle)
	verdict.EpistemicAction = ep.Action
	verdict.EpistemicReason = ep.Reason
	verdict.EpistemicSignals = append([]string(nil), ep.Signals...)
	verdict.EpistemicConfidence = ep.Confidence
	verdict.FailureMatches = ep.FailureMatches
	verdict.WeightedFailures = ep.WeightedFailures
	verdict.SchemaHash = ep.SchemaHash
	verdict.InputShape = ep.InputShape
	verdict.TemporalSensitive = ep.Temporal
	verdict.NovelTool = ep.NovelTool
	if reason, denied := agentToolPolicyDenial(agentToolPolicyFromCtx(ctx), tc.Name); denied {
		verdict.Allow = false
		verdict.Reason = reason
		verdict.WouldAsk = false
		verdict.HardDenied = true
		return verdict
	}
	if reason, denied := k.agentNoisePolicyDenial(ctx, tc); denied {
		verdict.Allow = false
		verdict.Reason = reason
		verdict.WouldAsk = false
		verdict.HardDenied = true
		return verdict
	}
	taint, hasTaint := agent.UntrustedObservationTaintFromContext(ctx)
	if hasTaint {
		verdict.UntrustedObservation = true
		verdict.ObservationSources = append([]string(nil), taint.Sources...)
		verdict.ObservationDirectiveLike = taint.DirectiveLike
		verdict.ObservationDirectiveMatches = append([]string(nil), taint.Matches...)
	}
	intentFrame, hasIntentFrame := intentmodel.FrameFromContext(ctx)
	intentAction := intentmodel.Action{
		ToolName:          tc.Name,
		Capability:        string(out.Capability),
		EffectClass:       bundle.EffectClass,
		Input:             string(tc.Input),
		AffectedResources: append([]string(nil), bundle.AffectedResources...),
	}
	regretAxes := intentmodel.RegretForAction(intentAction)
	confirmationPrompt := ""
	if hasIntentFrame {
		confirmationPrompt = intentmodel.ConfirmationPrompt(intentFrame, intentAction, regretAxes)
	}

	requiresApproval := out.RequiresApproval
	approvalReason := out.Reason
	if k.cfg.EpistemicEscalation && verdict.Allow && ep.escalates() {
		requiresApproval = true
		approvalReason = ep.Reason
		verdict.Allow = false
		verdict.WouldAsk = true
	}
	if k.cfg.IntentRegretGating && verdict.Allow && hasIntentFrame && intentmodel.RequiresConfirmation(intentFrame, regretAxes) {
		requiresApproval = true
		approvalReason = confirmationPrompt
		verdict.Allow = false
		verdict.WouldAsk = true
		k.publishIntentConfirmationRequired(correlationFromCtx(ctx), actorFromCtx(ctx), intentFrame, regretAxes, confirmationPrompt)
	}
	// Prompt-injection guard: an effectful action within the causal window of a
	// directive-like untrusted observation. The agent loop already scoped
	// taint.DirectiveLike to that window, so this no longer fires for the whole
	// run after one suspicious observation.
	if k.cfg.PromptInjectionGuard != PromptInjectionOff && verdict.Allow && hasTaint && taint.DirectiveLike && bundle.EffectClass != string(agent.EffectReadOnly) {
		// Block only in On mode and only when the operator hasn't trusted this
		// run; warn mode and a trusted run audit without interrupting.
		if k.cfg.PromptInjectionGuard == PromptInjectionOn && !trustedObservations(ctx) {
			requiresApproval = true
			approvalReason = "prompt-injection guard: effectful action is downstream of directive-like untrusted observation from " + strings.Join(taint.Sources, ", ")
			verdict.Allow = false
			verdict.WouldAsk = true
		} else {
			k.publishPromptInjectionWarned(correlationFromCtx(ctx), actorFromCtx(ctx), tc.Name, string(out.Capability), taint.Sources, trustedObservations(ctx))
		}
	}

	// Session-scoped operator grant (chat "auto-approve Tool Forge this session"):
	// if the run carries an auto-approve set covering this capability, satisfy the
	// approval without prompting and journal it as an auto-grant (WouldAsk stays
	// true so `agt why` shows it would have asked). Hard-denies never reach here
	// (they resolve to deny, not approval), so this can't override the F4 floor.
	if requiresApproval && autoApproveCap(ctx, string(out.Capability)) {
		verdict.Allow = true
		verdict.WouldAsk = true
		k.publishAutoApprove(correlationFromCtx(ctx), actorFromCtx(ctx), string(out.Capability), tc.Name)
		return verdict
	}

	if !requiresApproval {
		return verdict
	}

	// Live HITL: pause the tool-loop, route the request through the
	// approval queue, block until decided.
	actor := actorFromCtx(ctx)
	corr := correlationFromCtx(ctx)
	res := k.approvals.Submit(ctx, approval.SubmitSpec{
		Capability:            string(out.Capability),
		ToolName:              tc.Name,
		Input:                 string(tc.Input),
		Reason:                approvalReason,
		Actor:                 actor,
		CorrelationID:         corr,
		EffectClass:           bundle.EffectClass,
		PredictedEffects:      bundle.PredictedEffects,
		AffectedResources:     bundle.AffectedResources,
		RollbackNotes:         bundle.RollbackNotes,
		Confidence:            bundle.Confidence,
		CanonicalIntent:       intentFrame.CanonicalIntent,
		HarmfulInterpretation: intentFrame.HarmfulReading,
		AmbiguityScore:        intentFrame.AmbiguityScore,
		RegretAxes:            regretAxesPayload(regretAxes),
		ConfirmationPrompt:    confirmationPrompt,
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

func (k *Kernel) agentNoisePolicyDenial(ctx context.Context, tc agent.ToolCall) (string, bool) {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok {
		return "", false
	}
	if tc.Name == "memory" && policy.disableMemoryWrites && memoryToolActionWrites(tc.Input) {
		return "agent noise policy: memory writes are disabled", true
	}
	if tc.Name != "notify" {
		return "", false
	}
	if min := notifySeverityRank(policy.minNotifySeverity); min > notifySeverityRank(notifySeverityFromInput(tc.Input)) {
		return fmt.Sprintf("agent noise policy: notify severity must be at least %s", policy.minNotifySeverity), true
	}
	if policy.minNotifyIntervalSec <= 0 {
		return "", false
	}
	slug, _ := agentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return "", false
	}
	nowMS := time.Now().UnixMilli()
	var st agentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	if st.PendingNotifyMS > 0 && nowMS-st.PendingNotifyMS < int64(agentNoisePendingNotifyTTL/time.Millisecond) {
		return "agent noise policy: notify send already in progress", true
	}
	if st.LastNotifyMS > 0 {
		elapsed := nowMS - st.LastNotifyMS
		minMS := int64(policy.minNotifyIntervalSec) * 1000
		if elapsed < minMS {
			remaining := time.Duration(minMS-elapsed) * time.Millisecond
			return "agent noise policy: notify cooldown active for " + remaining.Round(time.Second).String(), true
		}
	}
	st.PendingNotifyMS = nowMS
	if err := k.state.Set(agentNoiseStateNS, slug, st); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	}
	return "", false
}

func (k *Kernel) completeAgentNoiseNotify(ctx context.Context, tc agent.ToolCall, res agent.Result) {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok || policy.minNotifyIntervalSec <= 0 || tc.Name != "notify" {
		return
	}
	slug, _ := agentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return
	}
	var st agentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	st.PendingNotifyMS = 0
	if !res.IsError {
		st.LastNotifyMS = time.Now().UnixMilli()
	}
	_ = k.state.Set(agentNoiseStateNS, slug, st)
}

func memoryToolActionWrites(raw json.RawMessage) bool {
	var in struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "remember", "forget", "bulk_forget":
		return true
	default:
		return false
	}
}

func notifySeverityFromInput(raw json.RawMessage) string {
	var in struct {
		Severity string `json:"severity"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return "info"
	}
	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if severity == "" {
		return "info"
	}
	return severity
}

func notifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info", "":
		return 1
	default:
		return 0
	}
}

type approvalBundle struct {
	EffectClass       string
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

func (k *Kernel) approvalDecisionBundle(toolName string, cap edict.Capability, input json.RawMessage, def agent.ToolDef) approvalBundle {
	effect := def.Effect
	if effect.Class == "" && len(effect.PredictedEffects) == 0 && len(effect.AffectedResources) == 0 {
		if tool, ok := k.tools[toolName]; ok {
			effect = tool.Definition().Effect
		}
	}

	class := normalizeEffectClass(effect.Class)
	if class == "" {
		class = defaultEffectClass(cap)
	}
	resources := append([]string(nil), effect.AffectedResources...)
	if len(resources) == 0 {
		resources = affectedResourcesFromInput(toolName, cap, input)
	}
	predicted := append([]string(nil), effect.PredictedEffects...)
	if len(predicted) == 0 {
		predicted = []string{fmt.Sprintf("invoke %s under %s", toolName, cap)}
	}
	rollback := strings.TrimSpace(effect.RollbackNotes)
	if rollback == "" {
		rollback = defaultRollbackNotes(class)
	}
	confidence := effect.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = defaultEffectConfidence(class)
	}
	return approvalBundle{
		EffectClass:       class,
		PredictedEffects:  predicted,
		AffectedResources: resources,
		RollbackNotes:     rollback,
		Confidence:        confidence,
	}
}

func normalizeEffectClass(class agent.EffectClass) string {
	switch class {
	case agent.EffectReadOnly, agent.EffectReversible, agent.EffectCompensable, agent.EffectIrreversible:
		return string(class)
	default:
		return ""
	}
}

func defaultEffectClass(cap edict.Capability) string {
	switch cap {
	case edict.CapFileRead, edict.CapFileList, edict.CapHTTPGet, edict.CapBrowserRead,
		edict.CapHomeAssistantRead, edict.CapWebSearch, edict.CapRunsRead,
		edict.CapIntrospect, edict.CapConfigRead, edict.CapProviderCall:
		return string(agent.EffectReadOnly)
	case edict.CapFileWrite, edict.CapMemory, edict.CapWorld, edict.CapSchedule,
		edict.CapStanding, edict.CapBoard, edict.CapSkill, edict.CapOversee,
		edict.CapToolForge, edict.CapConfigWrite, edict.CapWorkflow:
		return string(agent.EffectReversible)
	case edict.CapNotify, edict.CapHTTPPost, edict.CapRemoteRun:
		return string(agent.EffectCompensable)
	case edict.CapShell, edict.CapFileDelete, edict.CapCoding, edict.CapACPAgent,
		edict.CapHomeAssistantCall, edict.CapCodeExec, edict.CapMCPInstall, edict.CapMCP:
		return string(agent.EffectIrreversible)
	default:
		return string(agent.EffectIrreversible)
	}
}

func affectedResourcesFromInput(toolName string, cap edict.Capability, input json.RawMessage) []string {
	out := []string{"tool:" + toolName, "capability:" + string(cap)}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return out
	}
	for _, key := range []string{"path", "url", "endpoint", "entity_id", "service", "command", "op", "name", "id"} {
		if v, ok := obj[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, key+":"+s)
			}
		}
	}
	return out
}

func defaultRollbackNotes(class string) string {
	switch class {
	case string(agent.EffectReadOnly):
		return "No rollback required for read-only action."
	case string(agent.EffectReversible):
		return "Use the corresponding revert/delete/restore operation or journaled state to undo if needed."
	case string(agent.EffectCompensable):
		return "No guaranteed rollback; compensate with a follow-up action if the outcome is wrong."
	case string(agent.EffectIrreversible):
		return "No reliable rollback path declared; approve only if the effect is acceptable."
	default:
		return "No rollback information declared."
	}
}

func defaultEffectConfidence(class string) float64 {
	switch class {
	case string(agent.EffectReadOnly):
		return 0.95
	case string(agent.EffectReversible):
		return 0.75
	case string(agent.EffectCompensable):
		return 0.6
	case string(agent.EffectIrreversible):
		return 0.5
	default:
		return 0.4
	}
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
	ctxKeySystemAgent
	ctxKeyAgentLifecycle
	ctxKeyAgentRetryPolicy
	ctxKeyAgentToolPolicy
	ctxKeyAgentConfigOverrides
	ctxKeyAgentNoisePolicy
	ctxKeyWakeContext
	ctxKeyAutoApproveCaps
	ctxKeyTrustedObservations
)

// agentIdent carries a named agent's identity + daily ceiling for the
// Governor's per-agent ledger (M793).
type agentIdent struct {
	slug    string
	dailyMc int64
}

type agentToolPolicy struct {
	allow []string
	deny  []string
}

const agentNoiseStateNS = "agent_noise"

type agentNoisePolicy struct {
	silentOnSuccess      bool
	disableMemoryWrites  bool
	minNotifySeverity    string
	minNotifyIntervalSec int
}

type agentNoiseState struct {
	LastNotifyMS    int64 `json:"last_notify_ms"`
	PendingNotifyMS int64 `json:"pending_notify_ms,omitempty"`
}

const agentNoisePendingNotifyTTL = 5 * time.Minute

// WakeContext is durable provenance for why a run exists. It is stamped on
// task.received by agent.Run and intentionally kept separate from the prompt.
type WakeContext struct {
	Source            string
	Reason            string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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
// standing-order runner to bound an order's autonomy.
//
// The ceiling is monotonically TIGHTENING down a delegation tree: if the context
// already carries a tighter (lower) ceiling, that one is kept. A child run (e.g. a
// delegated sub-agent whose profile declares a looser TrustCeiling) can therefore
// never loosen the bound its parent was started with — only narrow it. Without
// this, WithAgentProfile re-applying a target profile's higher ceiling would
// overwrite a standing-order initiative cap and let delegation escape it
// (CWE-269, security finding VULN-001).
func WithTrustCeiling(ctx context.Context, ceiling edict.TrustLevel) context.Context {
	if ceiling >= edict.LevelAllow {
		// "No clamp" must not erase an existing tighter ceiling: leave ctx as-is so
		// any inherited cap survives.
		return ctx
	}
	if existing, ok := trustCeilingFromCtx(ctx); ok && existing < ceiling {
		ceiling = existing
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
// injection still layer on top, so a one-off identity/instruction override can
// be set per run without losing what Agezt knows.
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

// WithAutoApproveCapabilities marks a set of capabilities to auto-grant when
// the policy would otherwise prompt for HITL approval, for THIS run and every
// sub-agent it spawns (the context value rides the delegation tree). caps is a
// set of edict capability strings (e.g. {"tool.forge","code.exec"}). This is a
// session-scoped operator grant — e.g. the chat "auto-approve Tool Forge for
// this session" toggle when standing up an agent army — NOT a daemon-wide policy
// change. It never overrides a hard-deny (those resolve to deny, not approval).
// Empty leaves the context unchanged.
func WithAutoApproveCapabilities(ctx context.Context, caps map[string]bool) context.Context {
	if len(caps) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyAutoApproveCaps, caps)
}

// autoApproveCap reports whether capability c is in this run's auto-approve set.
func autoApproveCap(ctx context.Context, c string) bool {
	if v, ok := ctx.Value(ctxKeyAutoApproveCaps).(map[string]bool); ok {
		return v[c]
	}
	return false
}

// PromptInjectionMode selects how the prompt-injection guard handles an
// effectful action downstream of directive-like untrusted content.
type PromptInjectionMode int

const (
	// PromptInjectionOn routes the action to HITL approval (default).
	PromptInjectionOn PromptInjectionMode = iota
	// PromptInjectionWarn allows the action but journals a warning.
	PromptInjectionWarn
	// PromptInjectionOff disables the active intervention entirely.
	PromptInjectionOff
)

// ParsePromptInjectionMode maps an operator string (AGEZT_PROMPT_INJECTION_GUARD)
// to a mode. "off"/"0"/"false" → Off, "warn"/"warning"/"audit" → Warn, anything
// else (including empty) → On, preserving the historical on-by-default posture.
func ParsePromptInjectionMode(s string) PromptInjectionMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off", "0", "false", "no":
		return PromptInjectionOff
	case "warn", "warning", "audit":
		return PromptInjectionWarn
	default:
		return PromptInjectionOn
	}
}

// WithTrustedObservations marks a run as one whose untrusted-observation content
// the operator has chosen to trust (e.g. the chat "trust this run's web content"
// toggle). It downgrades the prompt-injection guard from blocking to warn FOR
// THIS RUN and its sub-agents, so a deliberately operator-driven agentic task
// isn't interrupted for every action — without changing the daemon-wide posture
// or touching any hard-deny. Never affects the F4 floor or SSRF/budget guards.
func WithTrustedObservations(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyTrustedObservations, true)
}

// trustedObservations reports whether this run carries the operator's
// trust-this-run grant.
func trustedObservations(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyTrustedObservations).(bool)
	return v
}

// WithAgentProfile applies a roster profile to a run's context (M790): the
// soul becomes the system override, the model + ordered fallbacks become the
// run's model chain, and the memory scope follows the identity (M786). The
// per-run cost ceiling is NOT applied here — callers layer it so their own
// explicit budget wins (mirrors handleRun's precedence). Used by the standing
// runner so an order can fire AS a named agent; handleRun keeps its inline
// application (its model resolves before the vision gate).
func WithAgentProfile(ctx context.Context, p roster.Profile) context.Context {
	noise := effectiveAgentNoisePolicy(p)
	if p.System {
		ctx = context.WithValue(ctx, ctxKeySystemAgent, true)
	}
	if sys := agentProfileSystem(p); sys != "" {
		ctx = WithSystem(ctx, sys)
	}
	if p.Lifecycle.Mode != "" || p.Lifecycle.RetireOnComplete {
		ctx = context.WithValue(ctx, ctxKeyAgentLifecycle, p.Lifecycle)
	}
	if p.RetryPolicy != nil {
		cp := *p.RetryPolicy
		cp.RetryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		ctx = context.WithValue(ctx, ctxKeyAgentRetryPolicy, cp)
	}
	if len(p.ToolAllow) > 0 || len(p.ToolDeny) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentToolPolicy, agentToolPolicy{
			allow: append([]string(nil), p.ToolAllow...),
			deny:  append([]string(nil), p.ToolDeny...),
		})
	}
	if noise != (agentNoisePolicy{}) {
		ctx = context.WithValue(ctx, ctxKeyAgentNoisePolicy, noise)
	}
	if len(p.ConfigOverrides) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentConfigOverrides, cloneStringMap(p.ConfigOverrides))
	}
	if ceiling := strings.TrimSpace(p.TrustCeiling); ceiling != "" {
		if lvl, err := edict.ParseTrustLevel(ceiling); err == nil {
			ctx = WithTrustCeiling(ctx, lvl)
		}
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

func effectiveAgentNoisePolicy(p roster.Profile) agentNoisePolicy {
	var out agentNoisePolicy
	if p.NoisePolicy != nil {
		out.silentOnSuccess = p.NoisePolicy.SilentOnSuccess
		out.disableMemoryWrites = p.NoisePolicy.DisableMemoryWrites
		out.minNotifySeverity = strings.ToLower(strings.TrimSpace(p.NoisePolicy.MinNotifySeverity))
		out.minNotifyIntervalSec = p.NoisePolicy.MinNotifyIntervalSec
	}
	if out.silentOnSuccess && notifySeverityRank(out.minNotifySeverity) < notifySeverityRank("warning") {
		out.minNotifySeverity = "warning"
	}
	if p.System {
		out.silentOnSuccess = true
		out.disableMemoryWrites = true
		if notifySeverityRank(out.minNotifySeverity) < notifySeverityRank("warning") {
			out.minNotifySeverity = "warning"
		}
		if out.minNotifyIntervalSec < 8*3600 {
			out.minNotifyIntervalSec = 8 * 3600
		}
	}
	return out
}

func agentNoisePolicyFromCtx(ctx context.Context) (agentNoisePolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentNoisePolicy).(agentNoisePolicy)
	return v, ok
}

func applyAgentNoisePolicyToPromptTools(tools map[string]agent.Tool, ctx context.Context) map[string]agent.Tool {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok || !policy.disableMemoryWrites {
		return tools
	}
	if _, ok := tools["memory"]; !ok {
		return tools
	}
	out := make(map[string]agent.Tool, len(tools)-1)
	for name, tool := range tools {
		if name != "memory" {
			out[name] = tool
		}
	}
	return out
}

func appendUniqueString(in []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	for _, x := range in {
		if strings.EqualFold(strings.TrimSpace(x), value) {
			return in
		}
	}
	return append(in, value)
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

// WithWakeContext attaches run provenance to the next agent loop. Empty fields
// are omitted from the journal; callers can layer it with WithAgentProfile.
func WithWakeContext(ctx context.Context, w WakeContext) context.Context {
	w.Source = strings.TrimSpace(w.Source)
	w.Reason = strings.TrimSpace(w.Reason)
	w.ScheduleID = strings.TrimSpace(w.ScheduleID)
	w.StandingID = strings.TrimSpace(w.StandingID)
	w.StandingName = strings.TrimSpace(w.StandingName)
	w.TriggerSubject = strings.TrimSpace(w.TriggerSubject)
	w.ParentCorrelation = strings.TrimSpace(w.ParentCorrelation)
	if w == (WakeContext{}) {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyWakeContext, w)
}

func wakeContextFromCtx(ctx context.Context) WakeContext {
	v, _ := ctx.Value(ctxKeyWakeContext).(WakeContext)
	return v
}

func systemAgentFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySystemAgent).(bool)
	return v
}

func agentToolPolicyFromCtx(ctx context.Context) agentToolPolicy {
	v, _ := ctx.Value(ctxKeyAgentToolPolicy).(agentToolPolicy)
	return v
}

func agentRetryPolicyFromCtx(ctx context.Context) (roster.RetryPolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentRetryPolicy).(roster.RetryPolicy)
	if !ok {
		return roster.RetryPolicy{}, false
	}
	v.RetryOn = append([]string(nil), v.RetryOn...)
	return v, true
}

// AgentConfigOverrides returns the named agent's config-override map attached to
// a run context by WithAgentProfile. The returned map is a copy and safe for the
// caller to mutate. Nil means the run carries no agent-specific overrides.
func AgentConfigOverrides(ctx context.Context) map[string]string {
	v, _ := ctx.Value(ctxKeyAgentConfigOverrides).(map[string]string)
	return cloneStringMap(v)
}

func agentConfigStringOverride(ctx context.Context, key string) (string, bool) {
	v, ok := agentConfigOverrideRaw(AgentConfigOverrides(ctx), key)
	if !ok {
		return "", false
	}
	return agentConfigStringValue(v)
}

func agentConfigBoolOverride(ctx context.Context, key string) (bool, bool) {
	raw, ok := agentConfigStringOverride(ctx, key)
	if !ok {
		return false, false
	}
	return agentConfigBoolValue(raw)
}

func agentConfigIntOverride(ctx context.Context, key string) (int, bool) {
	raw, ok := agentConfigStringOverride(ctx, key)
	if !ok {
		return 0, false
	}
	return agentConfigIntValue(raw)
}

func agentConfigDurationOverride(ctx context.Context, key string) (time.Duration, bool) {
	raw, ok := agentConfigStringOverride(ctx, key)
	if !ok {
		return 0, false
	}
	return agentConfigDurationValue(raw)
}

func agentProfileSystem(p roster.Profile) string {
	var b strings.Builder
	if soul := strings.TrimSpace(p.Soul); soul != "" {
		b.WriteString(soul)
	}
	if len(p.Instructions) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Standing instructions:\n")
		for _, ins := range p.Instructions {
			if ins = strings.TrimSpace(ins); ins != "" {
				b.WriteString("- ")
				b.WriteString(ins)
				b.WriteString("\n")
			}
		}
	}
	if len(p.TaskList) > 0 {
		cycle, total := profileTasksByScope(p.TaskList)
		if len(cycle) > 0 || len(total) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			if len(cycle) > 0 {
				b.WriteString("\nCycle tasks:\n")
				writeProfileTasks(&b, cycle)
			}
			if len(total) > 0 {
				b.WriteString("\nTotal tasks:\n")
				writeProfileTasks(&b, total)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func profileTasksByScope(tasks []roster.AgentTask) (cycle, total []roster.AgentTask) {
	for _, t := range tasks {
		status := strings.TrimSpace(t.Status)
		if status == "done" || status == "retired" {
			continue
		}
		if strings.TrimSpace(t.Scope) == "cycle" {
			cycle = append(cycle, t)
		} else {
			total = append(total, t)
		}
	}
	return cycle, total
}

func writeProfileTasks(b *strings.Builder, tasks []roster.AgentTask) {
	for i, t := range tasks {
		if i >= 20 {
			b.WriteString("- ...\n")
			return
		}
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		status := strings.TrimSpace(t.Status)
		if status == "" {
			status = "todo"
		}
		b.WriteString("- [")
		b.WriteString(status)
		b.WriteString("] ")
		b.WriteString(title)
		if desc := strings.TrimSpace(t.Description); desc != "" {
			b.WriteString(" - ")
			b.WriteString(desc)
		}
		b.WriteString("\n")
	}
}

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
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			keep[n] = struct{}{}
		}
	}
	out := make(map[string]agent.Tool, len(keep))
	for name, tool := range tools {
		if _, ok := keep[strings.ToLower(name)]; ok {
			out[name] = tool
		}
	}
	return out
}

func applyAgentToolPolicy(tools map[string]agent.Tool, pol agentToolPolicy) map[string]agent.Tool {
	out := tools
	if len(pol.allow) > 0 {
		out = filterTools(out, pol.allow)
	}
	if len(pol.deny) == 0 {
		return out
	}
	deny := make(map[string]struct{}, len(pol.deny))
	for _, name := range pol.deny {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			deny[name] = struct{}{}
		}
	}
	next := make(map[string]agent.Tool, len(out))
	for name, tool := range out {
		if _, blocked := deny[strings.ToLower(name)]; blocked {
			continue
		}
		next[name] = tool
	}
	return next
}

func agentToolPolicyDenial(pol agentToolPolicy, toolName string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return "", false
	}
	for _, denied := range pol.deny {
		if strings.ToLower(strings.TrimSpace(denied)) == name {
			return "agent tool denylist", true
		}
	}
	if len(pol.allow) == 0 {
		return "", false
	}
	for _, allowed := range pol.allow {
		if strings.ToLower(strings.TrimSpace(allowed)) == name {
			return "", false
		}
	}
	return "not in agent tool allowlist", true
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
	k.runsMu.Lock()
	defer k.runsMu.Unlock()
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
	k.runsMu.Lock()
	if k.halted {
		k.runsMu.Unlock()
		return
	}
	k.halted = true
	cancels := make([]context.CancelFunc, 0, len(k.runs))
	for _, c := range k.runs {
		cancels = append(cancels, c)
	}
	k.runs = make(map[string]context.CancelFunc)
	k.runsMu.Unlock()
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

// DrainAndHalt cancels all in-flight runs (equivalent to Halt()) and waits
// for them to unwind. It is the drain-phase primitive used by Close and by
// the self-update engine (M860). The timeout caps how long it waits; if
// exceeded, the function returns true (timedOut) with remaining runs still
// counted. A timeout of zero skips the drain wait entirely (cancels runs but
// does not wait).
//
// Use this instead of Halt() when the caller needs to know whether the drain
// completed within the timeout, e.g. for update vs. shutdown decisions.
func (k *Kernel) DrainAndHalt(timeout time.Duration) (timedOut bool, activeRuns int) {
	k.Halt() // cancel and mark halted; no-op if already halted
	k.runsMu.Lock()
	activeRuns = len(k.runs)
	k.runsMu.Unlock()
	if timeout <= 0 {
		return false, activeRuns
	}
	settled := make(chan struct{})
	go func() {
		k.runWG.Wait()
		close(settled)
	}()
	t := time.NewTimer(timeout)
	select {
	case <-settled:
		t.Stop()
		return false, 0
	case <-t.C:
		return true, activeRuns
	}
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
	k.runsMu.Lock()
	cancel, ok := k.runs[corr]
	if ok {
		delete(k.runs, corr)
	}
	k.runsMu.Unlock()
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
	k.runsMu.Lock()
	if !k.halted {
		k.runsMu.Unlock()
		return
	}
	k.halted = false
	k.runsMu.Unlock()
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
			if pol, ok := agentRetryPolicyFromCtx(ctx); ok && pol.MaxAttempts > 1 {
				return k.RunWithRetry(ctx, corr, task, pol)
			}
			return k.RunWith(ctx, corr, task)
		},
		func(ctx context.Context, task, answer string) (assure.Verdict, error) {
			return k.verifyCompletion(ctx, corr, task, answer)
		},
	)
	return res.Answer, res, err
}

// RunWithRetry executes one agent run using the profile's failure retry policy.
// This is distinct from provider retry (one LLM request) and RunAssured
// (semantic completion verification): it retries the whole governed run after a
// terminal error, journaling each retry decision under the same correlation.
func (k *Kernel) RunWithRetry(ctx context.Context, corr, intent string, pol roster.RetryPolicy) (string, error) {
	max := pol.MaxAttempts
	if max <= 1 {
		return k.RunWith(ctx, corr, intent)
	}
	if max > 10 {
		max = 10
	}
	var lastErr error
	for attempt := 1; attempt <= max; attempt++ {
		ans, err := k.RunWith(ctx, corr, intent)
		if err == nil {
			return ans, nil
		}
		lastErr = err
		reason := retryReason(err)
		if attempt >= max || !agentRetryable(reason, pol.RetryOn) {
			return "", err
		}
		delay := retryDelay(pol, attempt)
		agentSlug := agentSlugFromCtx(ctx)
		subject := "agent.retry"
		if agentSlug != "" {
			subject = "agent." + agentSlug + ".retry"
		}
		_, _ = k.bus.Publish(event.Spec{
			Subject:       subject,
			Kind:          event.KindAgentRetry,
			Actor:         "agent-retry",
			CorrelationID: corr,
			Payload: map[string]any{
				"agent":          agentSlug,
				"attempt":        attempt,
				"next_attempt":   attempt + 1,
				"max_attempts":   max,
				"reason":         reason,
				"error":          err.Error(),
				"delay_ms":       int64(delay / time.Millisecond),
				"backoff":        strings.TrimSpace(pol.Backoff),
				"base_delay_sec": pol.BaseDelaySec,
				"max_delay_sec":  pol.MaxDelaySec,
				"retry_on":       append([]string{}, pol.RetryOn...),
			},
		})
		if delay <= 0 {
			continue
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			return "", ctx.Err()
		case <-t.C:
		}
	}
	return "", lastErr
}

func retryReason(err error) string {
	switch {
	case errors.Is(err, ErrHalted):
		return "halted"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "error"
	}
}

func agentRetryable(reason string, retryOn []string) bool {
	if len(retryOn) == 0 {
		return reason == "error" || reason == "timeout"
	}
	for _, r := range retryOn {
		if strings.TrimSpace(r) == reason {
			return true
		}
	}
	return false
}

func retryDelay(pol roster.RetryPolicy, attempt int) time.Duration {
	base := time.Duration(pol.BaseDelaySec) * time.Second
	if base <= 0 {
		return 0
	}
	delay := base
	if strings.TrimSpace(pol.Backoff) == "exponential" {
		for i := 1; i < attempt; i++ {
			delay *= 2
		}
	}
	if pol.MaxDelaySec > 0 {
		max := time.Duration(pol.MaxDelaySec) * time.Second
		if delay > max {
			delay = max
		}
	}
	return delay
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
	k.runsMu.Lock()
	if k.halted {
		k.runsMu.Unlock()
		return "", ErrHalted
	}
	// Reject a correlation that is already running: two concurrent RunWith calls
	// sharing one id would clobber the run registry — the second's cancel overwrites
	// the first's k.runs[corr], and the first's deferred delete then removes the
	// second's entry, leaving a run uncancellable by Halt/CancelRun. The contract is
	// one id per run; enforce it instead of silently corrupting the registry. (M480)
	if _, running := k.runs[corr]; running {
		k.runsMu.Unlock()
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
	k.runsMu.Unlock()

	defer k.runWG.Done()
	defer func() {
		// Lock ordering: runsMu → fanoutMu → treeMu → steersMu → spawnsMu
		k.runsMu.Lock()
		delete(k.runs, corr)
		k.fanoutMu.Lock()
		delete(k.fanout, corr) // release this run's fan-out tally (M46)
		k.treeMu.Lock()
		delete(k.tree, corr) // release this tree's total sub-agent tally (M629)
		k.steersMu.Lock()
		delete(k.steers, corr) // release the steering control (M608)
		k.spawnsMu.Lock()
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
		k.spawnsMu.Unlock()
		k.steersMu.Unlock()
		k.treeMu.Unlock()
		k.fanoutMu.Unlock()
		k.runsMu.Unlock()
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
	systemAgent := systemAgentFromCtx(runCtx)
	skillDirective := skill.ParseActivationDirective(intent)
	if skillDirective.Explicit && skillDirective.CleanIntent != "" {
		intent = skillDirective.CleanIntent
	}
	intentFrame, ok := intentmodel.FrameFromContext(runCtx)
	if !ok {
		intentFrame = intentmodel.Interpret(intent)
		runCtx = intentmodel.WithFrame(runCtx, intentFrame)
	}
	k.publishIntentInterpreted(corr, actor, intentFrame)
	// So warden-backed tools (shell) stamp this run's correlation onto their
	// warden.executed events — making the isolation profile show up in the run's
	// timeline and walkable by `agt why`.
	runCtx = warden.WithCorrelation(runCtx, corr)

	disableHeuristicBypass := k.cfg.DisableHeuristicBypass
	if v, ok := agentConfigBoolOverride(runCtx, "AGEZT_DISABLE_HEURISTIC_BYPASS"); ok {
		disableHeuristicBypass = v
	}
	if !disableHeuristicBypass {
		if answer, ok := deterministicHeuristicBypass(intent, time.Now()); ok {
			if err := k.publishHeuristicBypass(runCtx, corr, actor, intent, answer); err != nil {
				return "", err
			}
			k.completeAgentLifecycle(runCtx, corr)
			return answer, nil
		}
	}

	// Memory injection: recall relevant records and prepend them to the
	// system prompt so the model starts the task already knowing what
	// Agezt remembers. The recall is journaled (memory.retrieved) under
	// corr, so `agt why` shows exactly what knowledge was surfaced.
	// Per-run system-prompt override (WithSystem): a one-off identity/instruction
	// set for this run only; falls back to the kernel's configured System. Memory /
	// world / skill injection below still layer on top.
	system := k.System() // live daemon default identity (M710), editable at runtime
	if s := systemFromCtx(runCtx); s != "" {
		system = s
	}
	// Operator profile (M1000): prepend what AGEZT has learned about the operator
	// so every (non-system) run knows WHO it works for — distinct from the per-agent
	// persona above (identity) and applied before the ephemeral task injections
	// below, so it sits adjacent to the persona. Always-on (not intent-driven);
	// a no-op until DistillProfile has synthesized a profile.
	if k.cfg.ProfileInject && !systemAgent {
		if p := k.memory.ProfileText(); p != "" {
			system = injectUserProfile(system, p)
		}
	}
	if k.cfg.MemoryInject && !systemAgent {
		topK := k.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		var candidates []contextCandidate
		if scored, err := k.memory.SearchScoped(intent, contextSelectionCandidateLimit, memory.ScopeFrom(runCtx)); err == nil {
			candidates = memoryContextCandidates(scored, time.Now().UnixMilli())
		}
		// Scoped to the run's agent identity (M786): a named agent's private
		// notes surface in its injected context; an unscoped run sees shared
		// memory only (RecallScoped with "" ≡ the previous Recall behaviour).
		if hits, err := k.memory.RecallScoped(corr, intent, topK, memory.ScopeFrom(runCtx)); err == nil && len(hits) > 0 {
			system = injectMemory(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Record.ID)
			}
			chosen, rejected := splitContextCandidates(candidates, chosenIDSet(ids), "memory_recall")
			k.publishContextSelection(corr, actor, contextSelectionManifest{
				Phase:    "memory",
				Query:    intent,
				Chosen:   chosen,
				Rejected: rejected,
				Summary:  candidateSummary(chosen, rejected),
			})
		}
	}

	// World-model injection: resolve the entities the intent refers to and
	// prepend them, so the model starts knowing what "the portfolio" means
	// (SPEC-05 §7 step 1). Resolve journals worldmodel.retrieved under corr,
	// so `agt why` shows what references were grounded.
	if k.cfg.WorldInject && !systemAgent {
		topK := k.cfg.WorldTopK
		if topK <= 0 {
			topK = 5
		}
		var candidates []contextCandidate
		if scored, err := k.world.ResolveQuiet(intent, contextSelectionCandidateLimit); err == nil {
			candidates = worldContextCandidates(scored, time.Now().UnixMilli())
		}
		if hits, err := k.world.Resolve(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectWorld(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Entity.ID)
			}
			chosen, rejected := splitContextCandidates(candidates, chosenIDSet(ids), "world_resolve")
			k.publishContextSelection(corr, actor, contextSelectionManifest{
				Phase:    "world",
				Query:    intent,
				Chosen:   chosen,
				Rejected: rejected,
				Summary:  candidateSummary(chosen, rejected),
			})
		}
	}

	// Skill activation: retrieve matching ACTIVE skills and prepend their
	// bodies so the model plans with learned procedures (SPEC-05 §4.2, §7
	// step 4). The pool is scoped to the acting agent (M932): shared skills
	// plus its own private ones. Activate journals skill.activated under corr
	// for `agt why`.
	var activatedSkillIDs []string
	if k.cfg.SkillInject && !systemAgent {
		topK := k.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		agentSlug := agentSlugFromCtx(runCtx)
		if skillDirective.Explicit {
			hits, missing, err := k.forge.ActivateExplicitFor(corr, agentSlug, intent, skillDirective.Refs, topK)
			if err == nil {
				if len(hits) > 0 {
					system = injectSkills(system, hits)
					for _, h := range hits {
						activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
					}
				}
				chosen := skillContextCandidates(hits, time.Now().UnixMilli())
				for i := range chosen {
					chosen[i].Chosen = true
					chosen[i].Reason = "selected:skill_explicit_activation"
				}
				summary := candidateSummary(chosen, nil)
				summary["activation"] = "explicit"
				summary["refs"] = skillDirective.Refs
				if len(missing) > 0 {
					summary["missing"] = missing
				}
				k.publishContextSelection(corr, actor, contextSelectionManifest{
					Phase:   "skill",
					Query:   intent,
					Chosen:  chosen,
					Summary: summary,
				})
			}
		} else {
			var candidates []contextCandidate
			if all, err := k.forge.List(); err == nil {
				pool := all[:0:0]
				for _, sk := range all {
					if sk.Agent == "" || sk.Agent == agentSlug {
						pool = append(pool, sk)
					}
				}
				candidates = skillContextCandidates(skill.Retrieve(pool, intent, contextSelectionCandidateLimit, time.Now().UnixMilli()), time.Now().UnixMilli())
			}
			if hits, err := k.forge.ActivateFor(corr, agentSlug, intent, topK); err == nil && len(hits) > 0 {
				system = injectSkills(system, hits)
				for _, h := range hits {
					activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
				}
				chosen, rejected := splitContextCandidates(candidates, chosenIDSet(activatedSkillIDs), "skill_activation")
				k.publishContextSelection(corr, actor, contextSelectionManifest{
					Phase:    "skill",
					Query:    intent,
					Chosen:   chosen,
					Rejected: rejected,
					Summary:  candidateSummary(chosen, rejected),
				})
			}
		}
	}

	// Per-run model override (WithModel) — used by the OpenAI-compatible API and
	// the Chat picker to honour the request's `model`. Falls back to the live
	// default model (k.Model(), hot-swappable via SetModel on provider reload —
	// M816).
	model := k.Model()
	modelExplicit := false
	if m := modelFromCtx(runCtx); m != "" {
		model = m
		modelExplicit = true
	} else if m, ok := agentConfigStringOverride(runCtx, "AGEZT_MODEL"); ok {
		model = m
	}
	// An EXPLICIT pick must actually serve the run (M931): the governor's
	// per-task chain ("chat" here) supersedes req.Model, so an operator choosing
	// a model in the Chat picker / `agt run --model` / the OpenAI-compat `model`
	// field was silently routed to the chain's models instead. Carry the pick as
	// the per-request chain, which wins over the task chain (M787 precedence). A
	// named agent's own chain (WithModelChain) still takes priority — it is the
	// more specific identity.
	modelChain := modelChainFromCtx(runCtx)
	if len(modelChain) == 0 && modelExplicit {
		modelChain = []string{model}
	}

	// Per-run tool restriction (WithTools): an allowlist (possibly empty = no
	// tools) scopes what this run may call, without changing the kernel's tool
	// set. Forged script tools (M794) and live MCP attachments (M796) are
	// merged BEFORE the filter so a restricted run only sees the dynamic
	// tools its allowlist grants.
	runTools := k.mergeMCPTools(k.mergeScriptTools(k.tools))
	runTools = applyAgentToolPolicy(runTools, agentToolPolicyFromCtx(runCtx))
	runTools = applyAgentNoisePolicyToPromptTools(runTools, runCtx)
	if allow, ok := toolsFromCtx(runCtx); ok {
		runTools = filterTools(runTools, allow)
	}
	toolDiscoveryMax := k.cfg.ToolDiscoveryMax
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_TOOL_DISCOVERY_MAX"); ok {
		toolDiscoveryMax = v
	}
	if toolDiscoveryMax > 0 && len(runTools) > toolDiscoveryMax {
		runTools = withToolSearch(runTools)
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
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_CONTEXT_BUDGET"); ok {
		ctxBudget = v
	}
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
		maxTok := elidedSummaryMaxTokens
		// A reasoning model needs headroom for its chain of thought or the
		// summary comes back empty (M926). The catalog knows which models
		// reason; locked accessor for the same M477 race reason as above.
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(model); m != nil && m.Reasoning {
				maxTok = elidedSummaryReasoningMaxTokens
			}
		}
		summarizeElided = makeElidedSummarizer(k.cfg.Provider, model, corr, maxTok)
	}

	maxIter := k.cfg.MaxIter
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_MAX_ITER"); ok {
		maxIter = v
	}
	maxAutoContinue := k.cfg.MaxAutoContinue
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_MAX_AUTO_CONTINUE"); ok {
		maxAutoContinue = v
	}
	autoContinueWait := k.cfg.AutoContinueWait
	if v, ok := agentConfigDurationOverride(runCtx, "AGEZT_AUTO_CONTINUE_WAIT"); ok {
		autoContinueWait = v
	}
	maxParallelTools := k.cfg.MaxParallelTools
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_PARALLEL_TOOLS"); ok {
		maxParallelTools = v
	}
	observationDeltas := k.cfg.ObservationDeltas
	if v, ok := agentConfigBoolOverride(runCtx, "AGEZT_OBSERVATION_DELTAS"); ok {
		observationDeltas = v
	}
	toolSelector := agent.LexicalToolSelector(toolDiscoveryMax)
	if _, ok := runTools[toolSearchName]; ok && toolDiscoveryMax > 0 {
		toolSelector = agent.DeferredLexicalToolSelector(toolDiscoveryMax, []string{toolSearchName})
	}
	wake := wakeContextFromCtx(runCtx)

	answer, err := agent.Run(runCtx, agent.LoopConfig{
		Provider:             k.cfg.Provider,
		Tools:                runTools,
		Bus:                  k.bus,
		Model:                model,
		TaskType:             "chat",     // M703: main agent loop → "chat" routing target
		ModelChain:           modelChain, // M787 agent fallbacks, or the explicit pick (M931)
		Agent:                agentSlugFromCtx(runCtx),
		AgentDailyCeilingMc:  agentDailyMcFromCtx(runCtx),
		WakeSource:           wake.Source,
		WakeReason:           wake.Reason,
		ScheduleID:           wake.ScheduleID,
		StandingID:           wake.StandingID,
		StandingName:         wake.StandingName,
		TriggerSubject:       wake.TriggerSubject,
		ParentCorrelation:    wake.ParentCorrelation,
		System:               system,
		MaxIter:              maxIter,
		MaxAutoContinue:      maxAutoContinue,  // M833: autonomous continue past MaxIter
		AutoContinueWait:     autoContinueWait, // M833
		ToolTimeout:          k.cfg.ToolTimeout,
		MaxParallelTools:     maxParallelTools, // M880: in-turn parallel tool dispatch
		Actor:                actor,
		CorrelationID:        corr,
		Policy:               k.policyHook,
		ToolSelector:         toolSelector,
		ToolResultHook:       k.completeAgentNoiseNotify,
		ObservationDeltas:    observationDeltas,
		ToolMemo:             agent.NewToolMemo(agent.DefaultToolMemoTTL, agent.DefaultToolMemoMaxEntries),
		Images:               imagesFromCtx(runCtx),   // M93: image attachments (vision-gated upstream)
		JSONMode:             jsonModeFromCtx(runCtx), // M314: structured-output request
		MaxRunCostMicrocents: maxCostFromCtx(runCtx),  // M166: per-run cost cap
		CostFn:               governor.CostMicrocents,
		Artifacts:            k.artifacts, // M390: offload oversized tool outputs (SPEC-04 §3.6)
		ArtifactThreshold:    k.cfg.ArtifactThreshold,
		ContextBudget:        ctxBudget,                 // M393/M394: context budgeting (SPEC-10 §3)
		ContextProtectFirst:  k.cfg.ContextProtectFirst, // M395: shield the earliest grounding
		SummarizeElided:      summarizeElided,           // M398: abstractive summary of dropped outputs
		ContextRescueMarkers: []string{agent.DefaultContextRescueMarker},
		Steer:                rc, // M608: live operator steering
	}, intent)

	// Deregister the steering control the instant the agent loop returns — BEFORE
	// the post-run work below (skill-outcome attribution, memory distillation,
	// which itself makes an LLM call). The outer defer also deletes it, but that
	// runs only after all post-processing; without this an operator pausing/
	// steering in that window would get a false success against a loop that has
	// already finished and will never Drain again (M608). delete is idempotent.
	k.steersMu.Lock()
	delete(k.steers, corr)
	k.steersMu.Unlock()

	// Attribute the run's outcome to the skills it activated, so an active skill
	// that repeatedly fails in production is auto-quarantined (SPEC-05 §5). This
	// is the production caller of RecordOutcome; best-effort bookkeeping that never
	// changes the run result.
	if k.forge != nil && len(activatedSkillIDs) > 0 {
		k.forge.RecordOutcome(corr, activatedSkillIDs, err == nil)
	}

	if err != nil {
		k.publishContextFailureAnalysis(corr, actor, err)
		return answer, err
	}

	// Auto-distillation: after a multi-tool run, extract durable facts
	// via one best-effort LLM call. Gated on a tool-call threshold so
	// simple Q&A runs aren't taxed with an extra round-trip. Failures are
	// journaled but never propagated — distillation must not turn a
	// successful task into a failed one.
	if k.cfg.MemoryDistill && !systemAgent {
		k.maybeDistill(runCtx, corr, intent, answer)
	}

	// Forge proposal: after a multi-tool run, propose a DRAFT skill via one
	// best-effort LLM call (the operator promotes it — §5.1/§5.3). Same
	// threshold-gated, never-fail-the-task contract as distillation.
	if k.cfg.SkillForge && !systemAgent {
		k.maybeForge(runCtx, corr, intent, answer)
	}
	// Shadow-evaluate relevant shadow skills against this completed run (SPEC-05
	// §5.2). We're past the err!=nil early return, so the run succeeded — a failed
	// run is a poor yardstick for "would it have helped".
	if k.cfg.ShadowEval && !systemAgent && k.forge != nil {
		k.maybeShadowEval(runCtx, corr, intent, answer)
	}
	k.completeAgentLifecycle(runCtx, corr)
	return answer, nil
}

func (k *Kernel) completeAgentLifecycle(ctx context.Context, corr string) {
	slug := agentSlugFromCtx(ctx)
	if slug == "" {
		return
	}
	current, ok := k.roster.Get(slug)
	if !ok || current.Retired {
		return
	}
	lifecycle := current.Lifecycle
	if shouldRetireAgentAfterComplete(lifecycle) {
		_, _ = k.SetProfileRetired(slug, true, "completed run "+corr)
		return
	}
	if strings.TrimSpace(lifecycle.Mode) != roster.LifecycleCycle && lifecycle.MaxCycles <= 0 {
		return
	}
	var completed, max int
	var advanced bool
	_, found, err := k.UpdateProfile(slug, func(p *roster.Profile) {
		// Idempotency: one logical run (correlation) advances the cycle exactly
		// once. RunAssured/RunWithRetry re-invoke RunWith under the SAME corr
		// (re-running until the work verifies complete / a transient error
		// clears); each inner success calls here, so without this guard a single
		// logical run would double-count. The check sits inside the atomic
		// UpdateProfile, so it is race-free, and the marker is durable across
		// restarts. A correlation-less completion (corr=="") is never guarded.
		if corr != "" && strings.TrimSpace(p.Lifecycle.LastCompletedRun) == corr {
			completed = p.Lifecycle.CompletedCycles
			max = p.Lifecycle.MaxCycles
			return
		}
		if strings.TrimSpace(p.Lifecycle.Mode) == "" {
			p.Lifecycle.Mode = roster.LifecycleCycle
		}
		p.Lifecycle.CompletedCycles++
		if corr != "" {
			p.Lifecycle.LastCompletedRun = corr
		}
		resetCompletedCycleTasks(p.TaskList)
		completed = p.Lifecycle.CompletedCycles
		max = p.Lifecycle.MaxCycles
		advanced = true
	})
	if err != nil || !found || !advanced {
		return
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "roster." + slug,
		Kind:          event.KindRosterUpdated,
		Actor:         "roster",
		CorrelationID: corr,
		Payload: map[string]any{
			"slug":             slug,
			"action":           "lifecycle_cycle_completed",
			"completed_cycles": completed,
			"max_cycles":       max,
			"run":              corr,
		},
	})
	if max > 0 && completed >= max {
		_, _ = k.SetProfileRetired(slug, true, fmt.Sprintf("completed %d/%d cycles on run %s", completed, max, corr))
	}
}

// CompleteAgentLifecycle advances the durable lifecycle for a successful
// non-loop job that ran under an agent profile, such as a scheduled workflow or
// direct tool target. RunWith calls the private helper itself; external runners
// should call this only after the job has genuinely succeeded.
func (k *Kernel) CompleteAgentLifecycle(ctx context.Context, corr string) {
	k.completeAgentLifecycle(ctx, corr)
}

func shouldRetireAgentAfterComplete(l roster.AgentLifecycle) bool {
	return l.RetireOnComplete || strings.TrimSpace(l.Mode) == roster.LifecycleRetireOnComplete
}

func resetCompletedCycleTasks(tasks []roster.AgentTask) {
	for i := range tasks {
		if strings.TrimSpace(tasks[i].Scope) == "cycle" && strings.TrimSpace(tasks[i].Status) == "done" {
			tasks[i].Status = "todo"
		}
	}
}

func deterministicHeuristicBypass(intent string, now time.Time) (string, bool) {
	q := strings.ToLower(strings.TrimSpace(strings.Trim(intent, " ?!.\t\r\n")))
	switch q {
	case "time", "current time", "what time is it", "what is the time", "saat kac", "saat kaç":
		return "Current time: " + now.Format(time.RFC3339), true
	case "date", "today", "today's date", "what is today's date", "bugunun tarihi", "bugünün tarihi":
		return "Current date: " + now.Format("2006-01-02"), true
	default:
		return "", false
	}
}

func (k *Kernel) publishHeuristicBypass(ctx context.Context, corr, actor, intent, answer string) error {
	subject := func(suffix string) string {
		return fmt.Sprintf("agent.%s.%s", actor, suffix)
	}
	publish := func(kind event.Kind, suffix string, payload any) error {
		_, err := k.bus.Publish(event.Spec{
			Subject:       subject(suffix),
			Kind:          kind,
			Actor:         actor,
			CorrelationID: corr,
			Payload:       payload,
		})
		return err
	}
	if err := publish(event.KindTaskReceived, "task", map[string]any{"intent": intent}); err != nil {
		return apperrors.Wrap(ctx, "runtime: publish heuristic task.received", err)
	}
	if err := publish(event.KindInfo, "heuristic", map[string]any{
		"bypass": "deterministic",
		"reason": "known-safe fast path",
	}); err != nil {
		return apperrors.Wrap(ctx, "runtime: publish heuristic bypass", err)
	}
	if err := publish(event.KindTaskCompleted, "task", map[string]any{
		"iters":   0,
		"chars":   len(answer),
		"stopped": "heuristic_bypass",
		"answer":  truncateHeuristicAnswer(answer),
	}); err != nil {
		return apperrors.Wrap(ctx, "runtime: publish heuristic task.completed", err)
	}
	return nil
}

func truncateHeuristicAnswer(s string) string {
	const max = 4096
	if len(s) <= max {
		return s
	}
	return s[:max] + "…[truncated]"
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

// elidedSummaryReasoningMaxTokens is the cap when the run's model is a
// reasoning model (M926): such models spend output tokens on their chain of
// thought BEFORE the summary line, so the tight cap gets entirely consumed and
// Complete returns empty content — observed live on deepseek-v4-pro at 64
// (every abstractive summary silently degraded to the extractive head stub).
// The prompt still asks for one line; the headroom is only used by models that
// actually reason.
const elidedSummaryReasoningMaxTokens = 1024

// elidedSummaryInputCap bounds how much of a dropped output is fed to the
// summarizer — enough to summarise, while keeping the summary call's own input
// (and therefore its cost) bounded regardless of how large the output was.
const elidedSummaryInputCap = 8 << 10

// makeElidedSummarizer builds the LoopConfig.SummarizeElided closure: a bounded,
// single-shot provider call that condenses a dropped tool output to one line
// (M398). It routes through the same provider (the Governor) as the run, so the
// extra call is billed and attributed to the run via corr. Errors propagate; the
// loop swallows them and falls back to the deterministic head snippet.
// maxTokens is caller-chosen: tight for plain models, roomy for reasoning
// models whose chain of thought eats the budget first (M926).
func makeElidedSummarizer(provider agent.Provider, model, corr string, maxTokens int) func(context.Context, string) (string, error) {
	return func(ctx context.Context, output string) (string, error) {
		in := output
		if len(in) > elidedSummaryInputCap {
			in = in[:elidedSummaryInputCap]
		}
		resp, err := provider.Complete(ctx, agent.CompletionRequest{
			Model:         model,
			CorrelationID: corr,
			TaskType:      "summarize",
			MaxTokens:     maxTokens,
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

// injectUserProfile prepends the learned operator profile (M1000) to the system
// prompt so the agent always knows who it works for. profileText is the
// pre-formatted facet block from memory.ProfileText (never empty here).
func injectUserProfile(system, profileText string) string {
	var b strings.Builder
	b.WriteString("What you know about the operator you work for (apply naturally; don't recite):\n")
	b.WriteString(profileText)
	b.WriteString("\n")
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
	if bias := forgeBias(tools); bias != "" {
		b.WriteString(bias)
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

// forgeBias nudges the agent toward DETERMINISM and SELF-IMPROVEMENT (M902): for
// work that must be exact, is repeatable, or you'll likely do again, prefer a
// tool over ad-hoc reasoning — write a script so the result is deterministic and
// re-runnable, forge a recurring script into a durable tool, and capture what
// works as a skill so it compounds across runs. Tuned to the tools present;
// returns "" when none of code_exec / tool_forge / skill is available (nothing
// to bias toward). Complements capabilityBriefing (which says what you CAN do)
// with how to work well.
func forgeBias(tools map[string]agent.Tool) string {
	_, hasCode := tools["code_exec"]
	_, hasForge := tools["tool_forge"]
	_, hasSkill := tools["skill"]
	if !hasCode && !hasForge && !hasSkill {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Prefer deterministic tools — and improve your own\n")
	b.WriteString("For work that must be exact, is repeatable, or you'll likely do again, reach for a tool instead of reasoning it out by hand each time:\n")
	if hasCode {
		b.WriteString("- Write a script (code_exec) so the result is deterministic, checkable, and re-runnable — not re-derived (and error-prone) each turn. Computation, parsing, transforms, and anything with exact rules belong in code.\n")
	}
	if hasForge {
		b.WriteString("- When a one-off script recurs, forge it into a durable tool (tool_forge) so the capability persists and the next run just calls it.\n")
	}
	if hasSkill {
		b.WriteString("- Check existing skills/tools before re-deriving one, and capture a working approach as a reusable skill (skill op=learn) so it's there next time.\n")
	}
	b.WriteString("Treat each run as self-improvement: when you hit a capability gap, build the tool that closes it — you finish faster and more reliably every time after.\n")
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
	// Normalize Windows separators first: on Linux filepath.Base treats
	// `C:\Win\cmd.exe` as one element and the interpreter would misroute
	// to the POSIX advice.
	switch strings.ToLower(filepath.Base(strings.ReplaceAll(shellBin, `\`, "/"))) {
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
