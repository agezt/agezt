// SPDX-License-Identifier: MIT

// Package runtime wires the kernel subsystems (journal + state + bus +
// agent loop + providers + tools) into a single Kernel that the daemon
// hosts and the control plane drives.
//
// Boundary note: runtime is the composition root and thin adapter layer for
// the running Agezt process. It may temporarily host orchestration helpers
// while boundaries are being extracted, but long-term feature-specific logic
// should live in narrower domain packages (delegation, workflow execution,
// tool execution, context selection, etc.) with runtime assembling and owning
// the services.
//
// One Kernel per Agezt process. Concurrent Run calls are allowed (each
// gets its own correlation_id and ctx); Halt cancels every in-flight run
// and prevents new ones until Resume.
package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime/accessors"
	"github.com/agezt/agezt/kernel/runtime/lifecycle"
	"github.com/agezt/agezt/kernel/runtime/runexec"
	"github.com/agezt/agezt/kernel/runtime/types"
	_ "github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/seat"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workboard"
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
type PluginInfo = types.PluginInfo

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

	// ResumeEnabled turns on durable run resume (M1002): a root run's dispatch
	// context and conversation snapshot are persisted so that if the daemon goes
	// down — stop/start, self-update, or hard kill — the run is re-dispatched on
	// restart and continues instead of being abandoned. Off leaves the historical
	// cancel-and-drop behaviour. The daemon defaults it on (default-allow posture).
	ResumeEnabled bool
	// ResumeSnapshotMaxBytes caps a serialized resume ticket; a larger snapshot is
	// dropped (the run then resumes by intent-replay). 0 uses the package default.
	ResumeSnapshotMaxBytes int

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

	// AutoApproveCapabilities is a daemon-wide operator grant for capabilities
	// that should not block in live HITL mode. It is applied to every run and
	// inherited by sub-agents. It satisfies only approvals raised by the Edict
	// Ask axis; it never overrides hard-deny, explicit tool-deny, SSRF, budgets,
	// or other fail-closed guards — including the prompt-injection guard,
	// epistemic escalation, and intent/regret gating, each of which routes to
	// live HITL regardless of this grant.
	AutoApproveCapabilities map[string]bool

	// AutoPromoteScriptTools lets a tested tool_forge draft go live immediately
	// when an agent requests promotion. The passing-test invariant remains: an
	// untested or failed draft is refused before promotion.
	AutoPromoteScriptTools bool

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
	// TasteInject prepends curated "what good looks like" exemplars (kernel/taste)
	// scoped to the run, so output quality is anchored to concrete examples. Gated
	// by AGEZT_TASTE_INJECT (default on); a no-op until exemplars exist.
	TasteInject bool
	// TasteTopK bounds how many exemplars are injected per run (default 3).
	TasteTopK int
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
	//   PromptInjectionWarn (default) — allow it, but journal a prompt_injection.warned
	//                         event so the chat can surface a passive banner.
	//   PromptInjectionOn   — route it to HITL approval.
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

	// CouncilWebSearch grounds the Council of Elders in current facts: before the
	// panel deliberates, the question is run through the `web_search` tool (from
	// cfg.Tools) and the top results are folded into a dated "research brief" every
	// seat — and the chair — see, alongside today's date. The daemon sets it from
	// AGEZT_COUNCIL_WEBSEARCH (default on). Off, or no web_search tool present, and
	// the council convenes with only the date (its prior behaviour, plus the date).
	CouncilWebSearch bool
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
	resume       *resume.Store // durable in-flight-run tickets for restart resume (M1002); nil when disabled
	roster       *roster.Store
	toolForge    *toolforge.Store
	mcpStore     *mcp.Store
	workflows    *workflow.Store
	workboard    *workboard.Store
	okr          *okr.Store
	taste        *taste.Store
	seat         *seat.Store
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

	// lifecycle is the run-lifecycle sub-manager extracted as a
	// separate Go sub-package on Day 12. See kernel/runtime/lifecycle
	// for the surface and the KernelAPI interface that wires it back
	// to *Kernel. Initialised in Open(); nil before that, so any
	// method that goes through it is safe to call only post-Open.
	lifecycle *lifecycle.Manager

	// accessors is the read-side sub-manager extracted as a separate
	// Go sub-package on Day 13. See kernel/runtime/accessors. As of
	// this commit only the simple store getters (Journal, Bus,
	// State, Edict, Warden, Approvals, Scheduler, Provider, Memory,
	// AgentGateway, Schedules) are routed through it; the rest of
	// the read surface lands in subsequent commits.
	accessors *accessors.Accessor

	// runexec is the run-engine sub-manager extracted as a separate
	// Go sub-package on Day 21. See kernel/runtime/runexec. As of
	// Day 22 only the entry points (Run / RunAssured / RunWith /
	// NewCorrelation / Why) are routed through the sub-package; the
	// 260-line RunWith body migrates in Day 23.
	runexec *runexec.Runner

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
	// suspending latches true when Suspend begins tearing the daemon down for a
	// restart (M1002). A run cancelled while this is set is treated as INTERRUPTED
	// (its ticket is kept for resume) rather than operator-cancelled (ticket
	// deleted). Never cleared — the process is on its way out.
	suspending atomic.Bool
	system     string                        // live daemon default identity / system prompt (M710); seeded from cfg.System, editable at runtime
	model      string                        // live default model id (M816); seeded from cfg.Model, hot-swapped on provider reload
	runs       map[string]context.CancelFunc // correlation_id → cancel
	fanout     map[string]int                // spawning correlation_id → sub-agents spawned (M46 fan-out bound)
	tree       map[string]int                // root correlation_id → total sub-agents in the tree (M629 total bound)
	steers     map[string]*runControl        // correlation_id → live-steering control surface (M608)
	spawns     map[string]*spawnHandle       // child correlation_id → pending/finished async delegation (M881)
	runWG      sync.WaitGroup                // in-flight runs + async spawn goroutines; Close drains it bounded (M883)

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

// Lifecycle API accessors (the kernel/runtime/lifecycle sub-package
// speaks to *Kernel through these; see kernel/runtime/lifecycle/api.go).
// They are intentionally narrow — every public method is a new piece
// of coupling between the kernel umbrella and the sub-package, and
// the whole point of the split is to *narrow* that coupling.

// Halted is the read side of the halt flag. Used by lifecycle.Manager
// under runsMu.
func (k *Kernel) Halted() bool { return k.halted }

// SetHalted is the write side. Used by lifecycle.Manager under runsMu.
func (k *Kernel) SetHalted(b bool) { k.halted = b }

// Runs returns the live correlation → cancel map. The Manager reads
// and mutates this under runsMu; the kernel's own RunWith also writes
// it, under the same mutex.
func (k *Kernel) Runs() map[string]context.CancelFunc { return k.runs }

// RunsMu exposes the runs mutex so the Manager can honour the lock
// order documented on the Kernel struct (configMu < runsMu < fanoutMu <
// treeMu < steersMu < spawnsMu < mcpMu).
func (k *Kernel) RunsMu() *sync.Mutex { return &k.runsMu }

// RunWG exposes the in-flight WaitGroup so the Manager can wait
// for cancelled runs to unwind (M883).
func (k *Kernel) RunWG() *sync.WaitGroup { return &k.runWG }

// Suspending returns the atomic.Bool that latches true during a
// graceful shutdown (M1002). The Manager reads it but never writes
// it — only kernel/resume.go's Suspend does that.
func (k *Kernel) Suspending() *atomic.Bool { return &k.suspending }

// ConfigMu exposes the config-mutex so the Accessor sub-package can
// honour the same lock order. Day 14.
func (k *Kernel) ConfigMu() *sync.Mutex { return &k.configMu }

// PublishBusEvent is the Accessor sub-package's chokepoint for
// Standing / Roster CRUD side-effects. Returns whatever the bus
// returns; the Accessor ignores the id. Day 15.
func (k *Kernel) PublishBusEvent(spec event.Spec) (*event.Event, error) {
	return k.bus.Publish(spec)
}

// ArtifactStore returns the content-addressed artifact store
// (SPEC-04 §3.6), where the loop offloads oversized tool
// outputs. Exposed for the runexec sub-package's KernelAPI
// (Day 22). The same pointer is reachable via the existing
// Artifacts() method.
func (k *Kernel) ArtifactStore() *artifact.Store { return k.artifacts }

// Runexec sub-package delegations (Day 22 split).
//
// The implementations live in kernel/runtime/runexec. These
// methods preserve the legacy *Kernel.Run / *Kernel.RunAssured
// / *Kernel.RunWith public surface so callers in controlplane,
// cmd/agezt, and the runtime tests keep compiling unchanged.
// Day 23 will move the 260-line RunWith body into the Runner.
//
// Note (Day 22): the legacy Run / RunAssured / RunWith /
// CompleteAgentLifecycle methods still live in runexec.go
// (their post-Day-11 location). They are NOT yet routed through
// the runexec sub-package's Runner; that move is Day 23. The
// public wrappers below are *preparation* for that move: once
// the body migrates into the Runner, these wrappers become
// the public surface and the runexec.go methods become internal.

// DescribeImages runs the vision SIDECAR (M821). The body lives
// in the runexec sub-package's Runner (Day 33); this wrapper
// preserves the legacy *Kernel.DescribeImages public surface.
// The Runner returns its own runexec.ErrNoVisionModel sentinel
// (separate identity, same text) — the wrapper translates to
// the canonical runtime.ErrNoVisionModel so external callers
// can errors.Is the canonical value.
func (k *Kernel) DescribeImages(ctx context.Context, corr string, images []string, hint string) (string, error) {
	desc, err := k.runexec.DescribeImages(ctx, corr, images, hint)
	if errors.Is(err, runexec.ErrNoVisionModel) {
		return desc, ErrNoVisionModel
	}
	return desc, err
}

// MaybeDistill folds the run's journal and, if the run made
// enough tool calls, runs one best-effort distillation. The body
// lives in the runexec sub-package's Runner (Day 32); this wrapper
// preserves the legacy *Kernel.MaybeDistill public surface.
func (k *Kernel) MaybeDistill(ctx context.Context, corr, intent, answer string) {
	k.runexec.MaybeDistill(ctx, corr, intent, answer)
}

// MaybeForge proposes a DRAFT skill. The body lives in the
// runexec sub-package's Runner (Day 32); this wrapper preserves
// the legacy *Kernel.MaybeForge public surface.
func (k *Kernel) MaybeForge(ctx context.Context, corr, intent, answer string) {
	k.runexec.MaybeForge(ctx, corr, intent, answer)
}

// MaybeShadowEval judges shadow skills. The body lives in the
// runexec sub-package's Runner (Day 32); this wrapper preserves
// the legacy *Kernel.MaybeShadowEval public surface.
func (k *Kernel) MaybeShadowEval(ctx context.Context, corr, intent, answer string) {
	k.runexec.MaybeShadowEval(ctx, corr, intent, answer)
}

// VerifyCompletion asks the model whether the given ANSWER
// fully accomplishes TASK. Returns an assure.Verdict with a
// complete/gap judgement. The body lives in the runexec
// sub-package's Runner (Day 33); this wrapper preserves the
// legacy *Kernel.VerifyCompletion public surface.
func (k *Kernel) VerifyCompletion(ctx context.Context, corr, task, answer string) (assure.Verdict, error) {
	return k.runexec.VerifyCompletion(ctx, corr, task, answer)
}

// SetupRunState is the public surface of the per-run registration
// logic. The Runner calls this in place of the inline runsMu +
// steersMu dance that RunWith used to do (Day 34). The method
// encapsulates:
//   - the halted check (returns ErrHalted if set)
//   - the duplicate-corr guard (returns "correlation already running")
//   - the tenant + auto-approve + timeout ctx decoration
//   - the k.runs[corr] = cancel registration
//   - the runWG.Add(1) drain accounting
//   - the steersMu-locked k.steers[corr] = newRunControl() registration
// The lock-ordering invariant `runsMu → steersMu` is documented
// on the Kernel struct and re-checked whenever this method
// changes; the live-steering slot acquisition sits AFTER
// runsMu is released (see runtime.go:~640-650 for the history).
//
// Returns the decorated run context, the cancel func (caller
// invokes on completion), the steer interface (caller wires
// into LoopConfig.Steer), and an error.
func (k *Kernel) SetupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error) {
	return k.setupRunState(corr, parentCtx)
}

// CleanupRunState is the public surface of the deferred 5-mutex
// cleanup. The Runner calls this in place of the inline deferred
// block that RunWith used to do. The method encapsulates the
// lock-ordering invariant `runsMu → fanoutMu → treeMu → steersMu
// → spawnsMu` and the runWG.Done() call. Returns the orphan
// cancel funcs (child spawns that were still in flight when the
// run finished); the caller invokes them, then invokes the
// runCtx's cancel func.
func (k *Kernel) CleanupRunState(corr string) []context.CancelFunc {
	return k.cleanupRunState(corr)
}

// DeregisterRunSteer removes the steer handle for corr without
// acquiring runsMu. Used post-run to free the steering slot the
// instant the agent loop returns — BEFORE the deferred
// CleanupRunState runs (which would also delete it, but later in
// the post-processing pipeline; an operator pausing/steering in
// that window would otherwise get a false success against a loop
// that has already finished, M608).
func (k *Kernel) DeregisterRunSteer(corr string) {
	k.deregisterRunSteer(corr)
}

// PublishIntentInterpreted publishes the intent.interpreted
// journal event. Wraps the private publishIntentInterpreted in
// intent.go.
func (k *Kernel) PublishIntentInterpreted(corr, actor string, frame intentmodel.Frame) {
	k.publishIntentInterpreted(corr, actor, frame)
}

// PublishContextFailureAnalysis publishes the context-failure
// journal event. Wraps the private publishContextFailureAnalysis
// in context_selection.go.
func (k *Kernel) PublishContextFailureAnalysis(corr, actor string, runErr error) {
	k.publishContextFailureAnalysis(corr, actor, runErr)
}

// PublishHeuristicBypass journals the deterministic-lookup hit
// (what-time-is-it / today's-date) so the run's timeline still
// shows the task, the bypass, and the canned answer. The body
// lives in the runexec sub-package's Runner (Day 33); this wrapper
// preserves the *Kernel-side access for non-Runner callers and
// keeps the KernelAPI contract coherent with the other publish*
// entries.
func (k *Kernel) PublishHeuristicBypass(ctx context.Context, corr, actor, intent, answer string) error {
	return k.runexec.PublishHeuristicBypass(ctx, corr, actor, intent, answer)
}

// PublishBus is the single chokepoint the lifecycle sub-package uses
// to emit kernel.halt / kernel.resume events. Returns whatever the
// bus returns; callers ignore the id.
func (k *Kernel) PublishBus(spec event.Spec) (*event.Event, error) {
	return k.bus.Publish(spec)
}

// Lifecycle sub-package delegations (Day 12 split).
//
// The implementations live in kernel/runtime/lifecycle. These
// methods preserve the legacy *Kernel.Halt / *Kernel.Resume public
// surface so callers in controlplane, cmd/agezt, cmd/agt, and the
// runtime tests keep compiling unchanged. The behaviour is identical
// to the pre-split bodies — they just live in a different file.

// IsHalted reports whether Run will refuse to start.
func (k *Kernel) IsHalted() bool { return k.lifecycle.IsHalted() }

// Halt cancels every in-flight run and prevents new ones.
func (k *Kernel) Halt() { k.lifecycle.Halt() }

// HaltWith is Halt plus a free-text reason recorded on kernel.halt.
func (k *Kernel) HaltWith(reason string) { k.lifecycle.HaltWith(reason) }

// DrainAndHalt cancels all in-flight runs and waits for them to
// unwind, bounded by timeout.
func (k *Kernel) DrainAndHalt(timeout time.Duration) (bool, int) {
	return k.lifecycle.DrainAndHalt(timeout)
}

// CancelRun cancels a single in-flight run by correlation id.
func (k *Kernel) CancelRun(corr string) bool { return k.lifecycle.CancelRun(corr) }

// Resume clears the halt flag, allowing new runs.
func (k *Kernel) Resume() { k.lifecycle.Resume() }

// ResumeWith is Resume plus a free-text reason recorded on
// kernel.resume.
func (k *Kernel) ResumeWith(reason string) { k.lifecycle.ResumeWith(reason) }

// ActiveRuns returns the number of in-flight Run / RunPlan
// invocations.
func (k *Kernel) ActiveRuns() int { return k.lifecycle.ActiveRuns() }

// ActiveRunIDs returns the correlation ids of the runs in flight
// right now, sorted for determinism.
func (k *Kernel) ActiveRunIDs() []string { return k.lifecycle.ActiveRunIDs() }

// NewCorrelation mints a fresh correlation ID suitable for RunWith.
func (k *Kernel) NewCorrelation() string { return k.lifecycle.NewCorrelation() }

// SubjectForRun returns the bus subject pattern that matches every
// event emitted by the agent.Run identified by corr.
func (k *Kernel) SubjectForRun(corr string) string {
	return k.lifecycle.SubjectForRun(corr)
}

// Accessors sub-package delegations (Day 13 split).
//
// These methods preserve the legacy *Kernel.Journal / *Kernel.Bus
// / etc. public surface. They read the underlying fields directly
// (rather than going through *accessors.Accessor) to avoid the
// recursion that would result from the Accessor dispatching back
// into *Kernel via the implicit KernelAPI interface. The Accessor
// itself remains in the kernel/runtime/accessors sub-package and
// is unit-tested independently; it lands its public body once the
// kernel→sub-package call graph is restructured to break the cycle
// (planned for the follow-up commits — the rest of the read
// surface lands in subsequent commits).

// Journal exposes the underlying journal for read-only inspection.
func (k *Kernel) Journal() *journal.Journal { return k.journal }

// Bus exposes the underlying bus for the control plane to attach subscribers.
func (k *Kernel) Bus() *bus.Bus { return k.bus }

// State returns the durable key/value store backing the kernel.
func (k *Kernel) State() *state.FileStore { return k.state }

// Edict exposes the policy engine for read/configure.
func (k *Kernel) Edict() *edict.Engine { return k.edict }

// Warden exposes the isolation engine.
func (k *Kernel) Warden() warden.Engine { return k.warden }

// Approvals exposes the HITL queue.
func (k *Kernel) Approvals() *approval.Registry { return k.approvals }

// Scheduler exposes the DAG executor.
func (k *Kernel) Scheduler() *scheduler.Executor { return k.scheduler }

// Provider exposes the live agent.Provider.
func (k *Kernel) Provider() agent.Provider { return k.cfg.Provider }

// Memory returns the memory-lite manager.
func (k *Kernel) Memory() *memory.Manager { return k.memory }

// AgentGateway returns the Agent Gateway for subprocess communication.
func (k *Kernel) AgentGateway() *agentgw.Gateway { return k.agentGW }

// Schedules returns the persistent typed schedule store.
func (k *Kernel) Schedules() *cadence.Store { return k.schedules }

// Tools returns the live in-process tool map.
func (k *Kernel) Tools() map[string]agent.Tool { return k.tools }

// ErrHalted is returned by Run when the kernel is in halt state.
var ErrHalted = errors.New("runtime: kernel is halted")
