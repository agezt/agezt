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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/imagetool"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/reranktool"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/seat"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/kernel/tenantctx"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/voicetool"
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

	// Every store opened below registers its Close here; fail() unwinds them in
	// reverse order. The previous hand-copied close cascades had already
	// diverged (late failure paths leaked the journal handle), so this is the
	// one place unwind order lives.
	var closers []interface{ Close() error }
	fail := func(prefix string, err error) (*Kernel, error) {
		for i := len(closers) - 1; i >= 0; i-- {
			_ = closers[i].Close()
		}
		return nil, fmt.Errorf("%s: %w", prefix, err)
	}

	j, err := journal.Open(filepath.Join(cfg.BaseDir, "journal"), journal.Options{})
	if err != nil {
		return nil, fmt.Errorf("runtime: journal: %w", err)
	}
	closers = append(closers, j)
	st, err := state.Open(filepath.Join(cfg.BaseDir, "state"))
	if err != nil {
		return fail("runtime: state", err)
	}
	closers = append(closers, st)
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
		return fail("runtime: memory", err)
	}
	closers = append(closers, mstore)
	mgr := memory.NewManager(mstore, kbus)
	if cfg.MemoryEmbedder != nil {
		mgr.SetEmbedder(cfg.MemoryEmbedder) // M884: provider embeddings opt-in
	}

	wstore, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
	if err != nil {
		return fail("runtime: worldmodel", err)
	}
	closers = append(closers, wstore)
	wgraph := worldmodel.NewGraph(wstore, kbus)

	skstore, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
	if err != nil {
		return fail("runtime: skills", err)
	}
	closers = append(closers, skstore)
	forge := skill.NewForge(skstore, kbus)
	// Wire the on-disk bundle store so skills can ship reference files + scripts
	// (agentskills.io shape, M847). Best-effort: a bundle-store failure leaves
	// skills body-only rather than failing daemon start.
	if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
		forge.SetBundles(bundles)
	}

	schedStore, err := cadence.OpenStore(filepath.Join(cfg.BaseDir, "cadence"))
	if err != nil {
		return fail("runtime: cadence", err)
	}

	// Content-addressed artifact store (SPEC-04 §3.6): the agent loop offloads
	// oversized tool outputs here so the journal stays small. Store-only — no bus.
	artStore, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		return fail("runtime: artifacts", err)
	}
	// Metadata index over the blob store (M822) — browsable/deletable entries
	// (inbound images, tool outputs). Failure here is non-fatal to the blob store
	// but we surface it so the operator knows the file-manager won't populate.
	artIndex, err := artifact.OpenIndex(artStore, filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		return fail("runtime: artifact index", err)
	}

	// Personal Data Lake (M834): file-based structured collections agents build
	// and share. Pure on-disk (no handle to close), so its error path just unwinds
	// the prior stores like the others.
	lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
	if err != nil {
		return fail("runtime: data lake", err)
	}
	// Seed the built-in Personal Data Lake collections (M835) — expenses, calendar,
	// tasks, notes, habits, bookmarks, contacts. Idempotent (EnsureCollection skips
	// existing ones) and best-effort: a seed hiccup must not block boot, and the
	// next start retries.
	_, _ = lake.SeedBuiltins("system")

	ststore, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
	if err != nil {
		return fail("runtime: standing", err)
	}

	// Durable in-flight-run tickets (M1002): opened only when resume is enabled so
	// a disabled daemon writes nothing. The store just prepares a directory, so a
	// failure here is unusual but still unwinds the stores opened above.
	var rsstore *resume.Store
	if cfg.ResumeEnabled {
		rsstore, err = resume.Open(filepath.Join(cfg.BaseDir, "resume"), cfg.ResumeSnapshotMaxBytes)
		if err != nil {
			return fail("runtime: resume", err)
		}
	}

	rstore, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
	if err != nil {
		return fail("runtime: roster", err)
	}

	tfstore, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
	if err != nil {
		return fail("runtime: toolforge", err)
	}

	mcpstore, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
	if err != nil {
		return fail("runtime: mcp", err)
	}

	wfstore, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		return fail("runtime: workflows", err)
	}

	wbstore, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		return fail("runtime: workboard", err)
	}

	okrstore, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
	if err != nil {
		return fail("runtime: okr", err)
	}

	tastestore, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
	if err != nil {
		return fail("runtime: taste", err)
	}

	seatstore, err := seat.OpenStore(filepath.Join(cfg.BaseDir, "seats"))
	if err != nil {
		return fail("runtime: seats", err)
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
	var mktTool *market.Tool
	if cfg.MarketTool {
		mktTool = market.NewTool()
		effTools["market"] = mktTool
	}
	// The voice tool needs the kernel's artifact store (bound just after k is
	// built) to persist synthesized audio.
	var vTool *voicetool.Tool
	if cfg.Voice != nil {
		vTool = voicetool.New(cfg.Voice)
		effTools["voice"] = vTool
	}
	// image_generate needs the artifact store too (bound just after k is built).
	var imgTool *imagetool.Tool
	if cfg.ImageGenerator != nil {
		imgTool = imagetool.New(cfg.ImageGenerator)
		effTools["image_generate"] = imgTool
	}
	if cfg.Reranker != nil {
		effTools["rerank"] = reranktool.New(cfg.Reranker)
	}

	catStore := catalog.NewStore(catDir)
	cat := cfg.Catalog
	if cat == nil {
		loaded, err := catStore.Load()
		if err != nil {
			return fail("runtime: catalog load", err)
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
		resume:       rsstore,
		roster:       rstore,
		toolForge:    tfstore,
		mcpStore:     mcpstore,
		mcpConns:     make(map[string]mcp.Conn),
		workflows:    wfstore,
		workboard:    wbstore,
		okr:          okrstore,
		taste:        tastestore,
		seat:         seatstore,
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
		mktTool.Manager = k.Market
	}
	if imgTool != nil {
		imgTool.SaveArtifact = k.artifacts.Put
	}
	if vTool != nil {
		vTool.SaveArtifact = k.artifacts.Put
	}

	// Config Center for agent SDK config access (M???)
	configCenter, err := configcenter.Open(configcenter.DefaultConfig(cfg.BaseDir))
	if err != nil {
		return fail("runtime: configcenter", err)
	}
	closers = append(closers, configCenter)
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
		return fail("runtime: resolve agentgw token secret", err)
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
	k.Suspend("close") // M1002: classify in-flight runs as resumable before Halt cancels them
	k.Halt()           // cancel any in-flight runs first
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
	k.Suspend("drain") // M1002: classify in-flight runs as resumable BEFORE cancelling them
	k.Halt()           // cancel and mark halted; no-op if already halted
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
	// Resume ticket ownership (M1002): the assure wrapper owns one ticket for the
	// whole objective; inner attempts (RunWith/RunWithRetry) reuse the corr and
	// skip creation. On resume the objective re-runs from attempt 1 (re-verification
	// is cheap), so no inner message snapshot is kept for an assured ticket.
	ctx, owns := k.claimResumeTicket(ctx, corr, intent, resume.KindAssured, maxAttempts)
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
	if owns {
		k.finalizeResumeTicket(corr, err)
	}
	return res.Answer, res, err
}

// RunWithRetry executes one agent run using the profile's failure retry policy.
// This is distinct from provider retry (one LLM request) and RunAssured
// (semantic completion verification): it retries the whole governed run after a
// terminal error, journaling each retry decision under the same correlation.
func (k *Kernel) RunWithRetry(ctx context.Context, corr, intent string, pol roster.RetryPolicy) (ans string, err error) {
	max := pol.MaxAttempts
	if max <= 1 {
		return k.RunWith(ctx, corr, intent)
	}
	if max > 10 {
		max = 10
	}
	// Resume ticket ownership (M1002): the retry wrapper owns one ticket across
	// all attempts; the inner RunWith calls reuse the corr and skip creation. The
	// deferred finalize reads the final named err, so a shutdown-interrupted retry
	// keeps its ticket while a genuine give-up deletes it.
	var owns bool
	ctx, owns = k.claimResumeTicket(ctx, corr, intent, resume.KindRetry, 0)
	if owns {
		defer func() { k.finalizeResumeTicket(corr, err) }()
	}
	var lastErr error
	for attempt := 1; attempt <= max; attempt++ {
		ans, err = k.RunWith(ctx, corr, intent)
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
	resp, err := k.completeAux(ctx, corr, "verify", agent.CompletionRequest{
		Model:     k.Model(),
		MaxTokens: assureVerifyMaxTokens,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: prompt}},
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
	resp, err := k.completeAux(ctx, corr, "vision", agent.CompletionRequest{
		Model:     model,
		MaxTokens: visionDescribeMaxTokens,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: prompt, Images: images}},
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
	if len(k.cfg.AutoApproveCapabilities) > 0 {
		ctx = WithAutoApproveCapabilities(ctx, mergeAutoApproveCapabilities(ctx, k.cfg.AutoApproveCapabilities))
	}

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
	// into the agent loop via LoopConfig.Steer below. k.steers is guarded by
	// steersMu everywhere else (controlFor, the deregistration paths, resume's
	// shutdown broadcast) — taking only runsMu here raced with them (caught by
	// -race under concurrent Runs). Lock order runsMu → steersMu is respected.
	rc := newRunControl()
	k.steersMu.Lock()
	k.steers[corr] = rc
	k.steersMu.Unlock()
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

	// What THIS run's config actually is: daemon-wide, plus the operator's live
	// edits, plus the running agent's own overrides (see effectiveConfig).
	ecfg := k.effectiveConfig(runCtx)

	if !ecfg.DisableHeuristicBypass {
		if answer, ok := deterministicHeuristicBypass(intent, time.Now()); ok {
			if err := k.publishHeuristicBypass(runCtx, corr, actor, intent, answer); err != nil {
				return "", err
			}
			k.completeAgentLifecycle(runCtx, corr)
			return answer, nil
		}
	}

	// System-prompt assembly: per-run override or live default, then
	// profile/taste/memory/world/skill injection (buildRunPrompt).
	system, activatedSkillIDs := k.buildRunPrompt(runCtx, corr, actor, intent, systemAgent, skillDirective)

	model, modelExplicit := resolveRunModel(runCtx, ecfg)
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

	// Governance- and capacity-shaped LoopConfig fields — shared verbatim with
	// executeSubAgent so root and delegated runs can never diverge on cost
	// accounting, compaction, or tool policy (LD-1).
	lc := k.buildLoopConfig(runCtx, corr, model)

	// Host-environment preamble (M609) — see injectHostEnvironment.
	system = k.injectHostEnvironment(system, lc.Tools)

	wake := wakeContextFromCtx(runCtx)

	// Durable resume (M1002): a root run owns a ticket unless a governed wrapper
	// (RunAssured/RunWithRetry) or the resumer already created one for this corr.
	// A message-bearing run (Kind=run, fresh or resumed) also refreshes its
	// conversation snapshot each iteration via the Checkpoint hook, and a resumed
	// run seeds the saved conversation so it continues where it left off.
	var ownedHere bool
	runCtx, ownedHere = k.claimResumeTicket(runCtx, corr, intent, resume.KindRun, 0)
	var resumeCheckpoint func(int, []agent.Message)
	var resumePriorMessages []agent.Message
	var resumeStartIter int
	if kind, owned := resumeOwnedKind(runCtx); ownedHere || (owned && kind == resume.KindRun) {
		resumeCheckpoint = k.resumeCheckpointFn(corr)
		if msgs, it, ok := resumeSeedFromCtx(runCtx); ok {
			resumePriorMessages, resumeStartIter = msgs, it
		}
	}

	// Run identity + root-run-only concerns layered on the shared base.
	lc.TaskType = "chat"       // M703: main agent loop → "chat" routing target
	lc.ModelChain = modelChain // M787 agent fallbacks, or the explicit pick (M931)
	lc.Agent = agentSlugFromCtx(runCtx)
	lc.AgentDailyCeilingMc = agentDailyMcFromCtx(runCtx)
	lc.WakeSource = wake.Source
	lc.WakeReason = wake.Reason
	lc.ScheduleID = wake.ScheduleID
	lc.StandingID = wake.StandingID
	lc.StandingName = wake.StandingName
	lc.TriggerSubject = wake.TriggerSubject
	lc.ParentCorrelation = wake.ParentCorrelation
	lc.System = system
	lc.Actor = actor
	lc.CorrelationID = corr
	lc.Images = imagesFromCtx(runCtx)                // M93: image attachments (vision-gated upstream)
	lc.JSONMode = jsonModeFromCtx(runCtx)            // M314: structured-output request
	lc.MaxRunCostMicrocents = maxCostFromCtx(runCtx) // M166: per-run cost cap
	lc.Steer = rc                                    // M608: live operator steering
	lc.Checkpoint = resumeCheckpoint                 // M1002: persist snapshot each iteration
	lc.PriorMessages = resumePriorMessages           // M1002: seed a resumed run's conversation
	lc.StartIter = resumeStartIter                   // M1002: continue iter numbering on resume
	answer, err := agent.Run(runCtx, lc, intent)

	// Resume ticket (M1002): clear it on a clean/failed/cancelled terminal, but
	// keep it if the run was interrupted by shutdown (finalizeResumeTicket honours
	// k.suspending). Only when THIS frame owns the ticket — a wrapper/resumer owns
	// it otherwise and clears it on its own return.
	if ownedHere {
		k.finalizeResumeTicket(corr, err)
	}

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
		return fmt.Errorf("runtime: publish heuristic task.received: %w", err)
	}
	if err := publish(event.KindInfo, "heuristic", map[string]any{
		"bypass": "deterministic",
		"reason": "known-safe fast path",
	}); err != nil {
		return fmt.Errorf("runtime: publish heuristic bypass: %w", err)
	}
	if err := publish(event.KindTaskCompleted, "task", map[string]any{
		"iters":   0,
		"chars":   len(answer),
		"stopped": "heuristic_bypass",
		"answer":  truncateHeuristicAnswer(answer),
	}); err != nil {
		return fmt.Errorf("runtime: publish heuristic task.completed: %w", err)
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
// in seq order (the M0.5 form of `agt why`). Delegates to the journal's
// provenance scan (Phase 3.1 C).
func (k *Kernel) Why(eventID string) ([]*event.Event, error) { return k.journal.Why(eventID) }

// Causes returns the causation ancestry of an event, root-first — the
// provenance walk SPEC-01 §7.1 describes. Delegates to the journal's
// provenance scan (Phase 3.1 C).
func (k *Kernel) Causes(eventID string) ([]*event.Event, error) { return k.journal.Causes(eventID) }

// ParentOf returns the lead run's correlation for a sub-agent run, or "" if
// childCorr was not spawned via delegation (M42). Delegates to the journal's
// provenance scan (Phase 3.1 C).
func (k *Kernel) ParentOf(childCorr string) string { return k.journal.ParentOf(childCorr) }

// Verify replays every event and confirms the BLAKE3 chain is intact.
// Returns nil on success.
func (k *Kernel) Verify() error {
	return k.journal.Verify()
}
