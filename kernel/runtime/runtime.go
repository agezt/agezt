// SPDX-License-Identifier: MIT

// Kernel composition root: Config struct + PluginInfo alias.
// Code extracted from runtime.go during the Day-41 god-file split. Public API unchanged.
package runtime


import (
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime/types"
	"github.com/agezt/agezt/kernel/toolforge"
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