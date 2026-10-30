// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel composition root: Config struct + PluginInfo alias. Code
//             extracted from runtime.go during the Day-41 god-file split. Public API
//             unchanged.

import (
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/seatapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/edict"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
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
	Provider llm.Provider

	// Tools are the in-process tools advertised to the model.
	Tools map[string]toolapi.Tool

	// NewToolInvoker binds an invocation service once per Open, with this
	// kernel's policy/audit ports. Nil preserves the standalone legacy pipeline.
	// Constructors bind dependencies without executing tools.
	NewToolInvoker toolpipeline.Factory

	// ScriptRunner executes forged script tools (M794) in the code-exec
	// sandbox. When set, every run is additionally offered the toolforge
	// store's ACTIVE scripts as callable `forge_<name>` tools; nil disables
	// the offering (drafting/testing then reports the forge unavailable).
	// The daemon wires the code_exec tool here — same warden isolation,
	// scrubbed env, and `code.exec` Edict gate as direct code execution.
	//
	// The PORT lives in the contract layer (kernel/contract/toolregapi — the
	// registry's script runner, W3.45): the host injects the executor and may
	// not name a module's type.
	ScriptRunner toolregapi.ScriptRunner

	// ToolForge is the script-tool store the host holds (W3.45) — the
	// contract's port, not a module's store. The daemon opens the file
	// (toolforge/scripttools.json) and hands it over; the kernel drives the
	// lifecycle through it and the tool registry receives the narrower read
	// port.
	//
	// Nil means "no forge": drafting, testing and promoting report that the
	// forge is unavailable rather than calling into nothing.
	ToolForge toolforgeapi.HostStore

	// MCPDialer spawns + handshakes one MCP server on attach (M796).
	//
	// It is a SEAM (W3.58): the implementation lives in the MCP module and
	// the kernel may not name it, so the composition root injects it. Nil
	// means NO dialer — the attach then refuses with a readable error rather
	// than calling into nothing, because a nil func value would panic at the
	// call. Tests inject fakes.
	MCPDialer mcpapi.Dialer

	// MCPHTTPDialer handshakes one REMOTE MCP server over Streamable HTTP on
	// attach (M904, #39) — used when a registration carries a URL instead of a
	// command. It is a SEAM like its stdio twin (W3.58): the composition root
	// injects it, and nil means no http dialer.
	MCPHTTPDialer mcpapi.HTTPDialer

	// MCPStore is the durable MCP-server registry — the contract's store port
	// (W3.58). The registry's FILE is the composition root's business
	// (`cmd/agezt` opens `mcp/servers.json` and hands the port over), and the
	// kernel never opens a registry of its own.
	//
	// A host that was given none holds a nil port: the registry reads answer
	// "no servers" and the registry writes refuse, rather than calling into
	// nothing.
	MCPStore mcpapi.Registry

	// MCPBuilder builds the MCP registry service over the dependencies the
	// host collected (W3.71).
	//
	// The registry's LIFECYCLE is the extensions module's: the module keeps
	// the operations, the live attachments and the bridge, and the daemon
	// keeps the names. The daemon hands over the BUILDER (it names the
	// module), and the kernel calls it with the store, the two dialers and
	// its own publisher — so every lifecycle transition journals through the
	// kernel's bus without either side naming the other's implementation.
	//
	// Nil means no service: the registry writes refuse and the reads answer
	// "no servers".
	MCPBuilder extapi.MCPRegistryBuilder

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

	// ConfigVault, when set, holds the config center's secret-rated values
	// (the daemon passes its encrypted vault). Nil keeps them in the 0600
	// entry files. A tenant kernel must NOT inherit the primary's: its keys
	// would share the primary vault's configcenter: namespace.
	ConfigVault configcenter.SecretStore

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

	// Catalog, if set, is used instead of loading from CatalogStore.
	// The daemon pre-loads the catalog so it can pick the primary
	// provider; passing it through here avoids a redundant disk read
	// and makes sure runtime and daemon see the same snapshot.
	//
	// The row is the contract's (W3.51): the kernel may not name a module.
	Catalog *catalogapi.Catalog

	// CatalogStore is the port for the daemon's catalog directory — the
	// contract's store port (W3.51). The daemon opens the directory and
	// hands it over; the kernel reloads the snapshot through it.
	//
	// Nil means "no catalog library": the write surfaces (sync, discovery,
	// a custom provider) refuse with "the catalog library is not available on
	// this daemon" instead of writing into nothing.
	CatalogStore catalogapi.Store

	// CatalogSyncer is the port that fetches a remote catalog — the
	// contract's syncer port (W3.51). The daemon builds it over the
	// netguard-screened client and hands it over; the kernel never fetches
	// anything itself.
	//
	// Nil means "no catalog syncer": the sync surfaces refuse with "the
	// catalog library is not available on this daemon".
	CatalogSyncer catalogapi.Syncer

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
	// TasteInject prepends curated "what good looks like" exemplars (kernel/modules/knowledge/taste)
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
	MemoryEmbedder memoryapi.Embedder

	// MemoryStore is the memory's FILE — the contract's store port (W3.57).
	// The daemon opens it (memory/memory.json) and hands it over; the kernel
	// never opens a memory file of its own.
	MemoryStore memoryapi.Store
	// MemoryBuilder builds the memory manager over the dependencies the host
	// collected. The daemon hands it over (it names the module); the kernel
	// calls it with the store, its own publisher, the embedder and the
	// structured-output path.
	MemoryBuilder memoryapi.Builder
	// MemoryGenerator is the structured-output path the distillers use. It
	// lives in another module, so it is injected rather than named (W3.57).
	MemoryGenerator memoryapi.Generator

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

	// The `market` tool is NOT a kernel knob (W3.46): the tool lives in the
	// market module and the kernel is a module — it may not name a module's
	// implementation. The composition root (cmd/agezt) builds the tool over the
	// contract's marketplace port and hands it over in Tools like any other
	// tool, so the kernel reaches no market package.

	// Voice, when non-nil, is the daemon's speech adapter (typically the
	// OpenAI-compatible voice adapter plugin) built from AGEZT_STT_* /
	// AGEZT_TTS_*. The kernel never picks an implementation.
	//
	// It reaches the kernel two ways: the daemon's composition root builds the
	// `voice` TOOL over it (the tool lives in its own package, and the kernel
	// may not name a module's implementation — W3.44), and the kernel answers
	// "is speech configured?" on the daemon's behalf through Voice(). Unset →
	// no voice tool.
	Voice Voice

	// ImageGenerator, when non-nil, is the daemon's image adapter (the
	// OpenAI-compatible image plugin) built from AGEZT_IMAGE_*. The daemon's
	// composition root builds the `image_generate` TOOL over it (W3.44).
	// Unset → no image tool. Generated images are saved as artifacts.
	ImageGenerator ImageGen
	// Reranker, when non-nil, is the daemon's rerank adapter (the
	// Cohere/Jina-style rerank plugin) built from AGEZT_RERANK_*. The daemon's
	// composition root builds the `rerank` TOOL over it (W3.44). Unset → no
	// rerank tool.
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
	// and shared runtime invoker (direct/workflow/reasoning/code tool paths)
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
	// panel deliberates, the question is run through the registered `web_search`
	// tool via the governed invoker, and the top results form a "research brief" every
	// seat — and the chair — see, alongside today's date. The daemon sets it from
	// AGEZT_COUNCIL_WEBSEARCH (default on). Off, or no web_search tool present, and
	// the council convenes with only the date; policy refusal/search failure also
	// preserves this date-only behavior.
	CouncilWebSearch bool

	// Seats is the execution-seat catalog the host holds — the contract's port,
	// not a module's store (W3.41). The daemon opens the store and hands it over
	// (cmd/agezt), because the file is the daemon's business and the kernel is a
	// module: a module may not name another module's implementation.
	//
	// Nil means "no seat catalog": the seat operations report that the catalog
	// is unavailable, and no dispatch may run under a seat. It never means
	// "every seat is valid".
	Seats seatapi.Store

	// Lake is the personal data lake the host holds — the contract's port, not
	// a module's lake (W3.42). The daemon opens the lake, seeds its built-in
	// collections and hands it over (cmd/agezt): the lake's files are the
	// daemon's business, and the kernel is a module.
	//
	// Nil means "no data lake": the data operations report that the lake is
	// unavailable, and the db tool is handed a nil port.
	Lake datalakeapi.Lake

	// Artifacts is the content-addressed blob store the host holds, and
	// ArtifactIndex is the browsable metadata index over it (M822) — the
	// contract's ports, not a module's store (W3.43). The daemon opens both
	// and hands them over (cmd/agezt): the files are the daemon's business.
	//
	// The index is the HOST-facing port: the kernel counts the stale entries,
	// reclaims them and installs the journal's ref guard on it. The tool
	// registry receives the narrower tool-facing port.
	//
	// Nil means "no artifact store": the loop cannot offload an oversized
	// output, the tools cannot list or read artifacts, and the workboard's
	// proof gathers no artifacts — each of them says so rather than calling
	// into nothing.
	Artifacts     artifactapi.Blob
	ArtifactIndex artifactapi.HostIndex

	// Workboard is the durable task board the host holds — the contract's host
	// port, not a module's store (W3.47). The daemon opens the store file and
	// hands it over (cmd/agezt): the board's file is the daemon's business,
	// and the kernel is a module.
	//
	// The port is the HOST-facing one: the read side (Get, List) plus the
	// eighteen lifecycle verbs the host drives. The agent-facing tool keeps
	// the narrower read port.
	//
	// Nil means "no task board": the board operations report that the board is
	// unavailable, and the OKR rollup counts no linked task as done.
	Workboard workboardapi.HostStore

	// Workflows is the durable workflow store the host holds — the contract's
	// HOST port, not a module's store (W3.48). The daemon opens the store file
	// and hands it over (cmd/agezt): the store's file is the daemon's
	// business, and the kernel is a module.
	//
	// It is the host port and not the read side because the composition root
	// builds the ENGINE over this very store (SetWorkflowEngine): the engine
	// needs the host's capabilities and only the host has them. The kernel's
	// own face stays narrow — `Workflows()` hands out the read side only
	// (Get, List), so the agent-facing tool can never remove or disable a
	// stored workflow.
	//
	// Nil means "no workflow store": the reads answer that there is nothing
	// stored, and every lifecycle operation refuses.
	Workflows workflowapi.HostStore

	// Schedules is the typed schedule library the host holds — the contract's
	// HOST port, not a module's store (W3.49). The daemon opens the file
	// (cadence/schedules.json) and hands it over (cmd/agezt): the library's
	// file is the daemon's business, and the kernel is a module.
	//
	// The RESIDENT is a separate thing (SetScheduleEngine): the live cadence
	// engine is built by the composition root over this very store and over
	// the host's own bus, and it is handed over after Open.
	//
	// Nil means "no schedule library": the schedule surfaces report that the
	// library is unavailable rather than calling into nothing.
	Schedules cadenceapi.HostStore

	// Standing is the standing-order library the host holds — the contract's
	// store port, not a module's store (W3.50). The daemon opens the file
	// (standing/standing.json) and hands it over (cmd/agezt): the library's
	// file is the daemon's business, and the kernel is a module.
	//
	// The daemon's OWN residents (the event runner and the cron ticker) are
	// built over this very store by the composition root and handed over
	// afterwards — the kernel holds the port, not the residents.
	//
	// Nil means "no standing library": the standing surfaces report that the
	// library is unavailable rather than calling into nothing.
	Standing standingapi.Store

	// TasteStore is the curated exemplar library the host holds — the
	// contract's store port, not a module's store (W3.52). The daemon opens
	// the file (taste/taste.json) and hands it over (cmd/agezt): the
	// library's file is the daemon's business, and the kernel is a module.
	//
	// Nil means "no exemplar library": the kernel injects no exemplars into a
	// run rather than calling into nothing.
	TasteStore tasteapi.Store

	// OKRStore is the objective library the host holds — the contract's store
	// port, not a module's store (W3.53). The daemon opens the file
	// (okr/okr.json) and hands it over (cmd/agezt): the library's file is the
	// daemon's business, and the kernel is a module.
	//
	// Nil means "no objective library": the objective surfaces report that the
	// library is unavailable rather than calling into nothing.
	OKRStore okrapi.Store

	// WorldStore is the world-model file the host holds — the contract's
	// store port, not a module's store (W3.54). The daemon opens the file
	// (worldmodel/worldmodel.json) and hands it over (cmd/agezt): the
	// library's file is the daemon's business, and the kernel is a module.
	//
	// Nil means "no world model": the world surfaces report that the graph is
	// unavailable rather than calling into nothing.
	WorldStore worldmodelapi.Store

	// WorldGraph builds the world-model graph over the store and the kernel's
	// publisher.
	//
	// The DAEMON supplies the builder — it names the module — and the kernel
	// hands over the store it was given and its own publisher, so the graph
	// journals through the kernel's bus without either side naming the
	// other's implementation (W3.54).
	WorldGraph func(store worldmodelapi.Store, pub worldmodelapi.Publisher) worldmodelapi.Graph

	// SkillStore is the skill library's file the host holds — the contract's
	// store port, not a module's store (W3.55). The daemon opens the file
	// (skills/skills.json) and hands it over (cmd/agezt): the library's file
	// is the daemon's business, and the kernel is a module.
	//
	// Nil means "no skill library": the skill surfaces report that the library
	// is unavailable rather than calling into nothing.
	SkillStore skillapi.Store

	// SkillCurator builds the skill curator over the store and the kernel's
	// publisher.
	//
	// The DAEMON supplies the builder — it names the module — and the kernel
	// hands over the store it was given and its own publisher, so every
	// transition journals through the kernel's bus without either side naming
	// the other's implementation (W3.55).
	SkillCurator func(store skillapi.Store, pub skillapi.Publisher) skillapi.Curator

	// Roster is the roster file the host holds — the contract's store port,
	// not a module's store (W3.56). The daemon opens the file
	// (roster/roster.json) and hands it over (cmd/agezt): the roster's file
	// is the daemon's business, and the kernel is a module.
	//
	// Nil means "no roster": the roster reads answer "no agents" and the
	// roster writes refuse, rather than calling into nothing.
	Roster agentapi.Store
}

// Kernel is the running Agezt instance.
