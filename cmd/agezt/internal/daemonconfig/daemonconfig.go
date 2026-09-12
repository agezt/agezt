// SPDX-License-Identifier: MIT

// Daemon config types + tiny helpers (voiceProviderIsNative + splitNonEmpty).
// Code extracted from daemonconfig.go during the Day-94 god-file split.
// Public API unchanged.
package daemonconfig


import (
	"time"

	"github.com/agezt/agezt/kernel/edict"
)


// Config is every post-inject AGEZT_* boot setting runDaemon consumes, grouped
// by the natural clusters of the old inline code.
type Config struct {
	Policy    Policy
	Knowledge Knowledge
	RunLoop   RunLoop
	SubAgents SubAgents
	Context   ContextBudget
	Sidecars  Sidecars
	Guards    Guards
	Tenancy   Tenancy
	Lifecycle Lifecycle
	Misc      Misc
}

// Policy groups the Edict/approval settings (M611/M17/M20/M100).
type Policy struct {
	// AllowAll is AGEZT_ALLOW_ALL == "1": every governed capability at L4.
	AllowAll bool
	// EdictDeny is the operator-extensible hard-deny extras parsed from
	// AGEZT_EDICT_DENY (nil when unset). A malformed spec is a Load error.
	EdictDeny []edict.HardDenyRule
	// EdictDurable is AGEZT_EDICT_DURABLE == "on" at boot: replay journaled
	// policy.changed events onto the fresh engine. The tenant lazy-open path
	// re-reads the env live (see the package comment).
	EdictDurable bool
	// ApprovalTimeout is AGEZT_APPROVAL_TIMEOUT: how long a prompt-mode HITL
	// approval blocks before auto-deny. 0 = kernel default (5m). Malformed is
	// a Load error; a non-positive duration means "use default" (stays 0).
	ApprovalTimeout time.Duration
}

// Knowledge groups the memory / world-model / skills switches (ROADMAP §2.3,
// SPEC-05, M993/M1000).
type Knowledge struct {
	// Memory is AGEZT_MEMORY != "off": per-run inject/tool/distill.
	Memory bool
	// MemoryDistillMinTools is AGEZT_MEMORY_DISTILL_MIN_TOOLS (default 6;
	// malformed or non-positive silently falls back to the default).
	MemoryDistillMinTools int
	// UserProfile is Memory && AGEZT_USER_PROFILE != "off" (M1000).
	UserProfile bool
	// TasteInject is AGEZT_TASTE_INJECT != "off".
	TasteInject bool
	// WorldModel is AGEZT_WORLDMODEL != "off".
	WorldModel bool
	// Skills is AGEZT_SKILLS != "off" (skill injection).
	Skills bool
	// Forge is AGEZT_FORGE != "off" (post-run skill proposal).
	Forge bool
	// SkillShadowEval is AGEZT_SKILL_SHADOWEVAL == "on" (SPEC-05 §5.2).
	SkillShadowEval bool
	// SkillAutoQuarantine is AGEZT_SKILL_AUTOQUARANTINE != "off" (default on).
	SkillAutoQuarantine bool
	// SkillAutoShadow is AGEZT_SKILL_AUTOSHADOW == "on" (default off).
	SkillAutoShadow bool
	// SkillAutoPromote is AGEZT_SKILL_AUTOPROMOTE != "off" (default on).
	SkillAutoPromote bool
}

// RunLoop groups the per-run execution caps (M31/M824/M833/M880/M34/CH-03).
// Every malformed value here was a hard startup error inline and is a Load
// error now.
type RunLoop struct {
	// RunTimeout is AGEZT_RUN_TIMEOUT: per-run wall-clock cap. 0 = off
	// (a valid but non-positive duration also disarms).
	RunTimeout time.Duration
	// MaxIter is AGEZT_MAX_ITER: tool-call rounds per run. 0 = agent default.
	MaxIter int
	// MaxAutoContinue / MaxAutoContinueSet carry AGEZT_MAX_AUTO_CONTINUE.
	// The env accepts any integer (negative disables auto-continue), so a
	// separate Set flag distinguishes "unset" from an explicit 0.
	MaxAutoContinue    int
	MaxAutoContinueSet bool
	// AutoContinueWait is AGEZT_AUTO_CONTINUE_WAIT (non-negative duration).
	AutoContinueWait time.Duration
	// MaxParallelTools is AGEZT_PARALLEL_TOOLS. 0 = agent default.
	MaxParallelTools int
	// ToolDiscoveryMax is AGEZT_TOOL_DISCOVERY_MAX. 0 = off (also the
	// explicit-"0" value; both leave the kernel offering every tool).
	ToolDiscoveryMax int
	// ToolTimeout is AGEZT_TOOL_TIMEOUT: per-tool-call cap. 0 = off.
	ToolTimeout time.Duration
}

// SubAgents groups the delegation rails (P6-MULTI-01, M843/M46/M48/M629).
type SubAgents struct {
	// Enabled is AGEZT_SUBAGENT != "off".
	Enabled bool
	// Depth is AGEZT_SUBAGENT_DEPTH (default 3; malformed/non-positive
	// silently falls back).
	Depth int
	// Fanout is AGEZT_SUBAGENT_FANOUT (0 = unbounded).
	Fanout int
	// SpendCapMicrocents is AGEZT_SUBAGENT_SPEND_CAP (USD) × 1e9. Malformed
	// or negative is a Load error. 0 = unbounded.
	SpendCapMicrocents int64
	// MaxTotal is AGEZT_SUBAGENT_MAX_TOTAL, with the M843 derivation applied:
	// unset (0) while Depth > 1 becomes 48 so deep delegation stays bounded.
	MaxTotal int
}

// ContextBudget groups the context-assembly caps (SPEC-04 §3 / SPEC-10 §3).
// Malformed values here warn and fall back (never fatal).
type ContextBudget struct {
	// ArtifactThreshold is AGEZT_ARTIFACT_THRESHOLD (bytes). 0 = kernel
	// default (agent.DefaultArtifactThreshold).
	ArtifactThreshold int
	// Budget / BudgetAuto carry AGEZT_CONTEXT_BUDGET: a positive char count,
	// or "auto" to derive from the model's catalog context window.
	Budget     int
	BudgetAuto bool
	// ProtectFirst is AGEZT_CONTEXT_PROTECT_FIRST (M395). 0 = oldest-first.
	ProtectFirst int
	// Summarize is AGEZT_CONTEXT_SUMMARIZE == "1" (M398).
	Summarize bool
}

// Sidecars groups the five near-identical OpenAI-compatible model-service
// clusters (M901 embeddings, M997 image + rerank, M998 voice).
type Sidecars struct {
	Embed  ModelService // AGEZT_EMBED_URL / _MODEL / _KEY
	Image  ModelService // AGEZT_IMAGE_URL / _MODEL / _KEY
	Rerank ModelService // AGEZT_RERANK_URL / _MODEL / _KEY
	STT    VoiceHalf    // AGEZT_STT_PROVIDER / _URL / _MODEL / _KEY
	TTS    VoiceHalf    // AGEZT_TTS_PROVIDER / _URL / _MODEL / _KEY / _VOICE
}

// ModelService is one URL+model+key sidecar. When URL is set but Model is
// empty, Load warns ("… disabled") and Enabled() reports false — the service
// degrades, never fails the boot.
type ModelService struct {
	URL   string
	Model string
	Key   string
}

// Enabled reports whether the sidecar should be constructed (URL and model
// both present).
func (s ModelService) Enabled() bool { return s.URL != "" && s.Model != "" }

// VoiceHalf is one STT or TTS half. Native providers (ElevenLabs, Deepgram,
// Cartesia) supply their own base URL, so URL is optional for them; Attempt
// carries the historical gate: (URL set OR native provider) AND NOT
// (model empty on a non-native provider — which warns instead).
type VoiceHalf struct {
	Provider string
	URL      string
	Model    string
	Key      string
	// Voice is the TTS voice id (AGEZT_TTS_VOICE); unused for STT.
	Voice string
	// Attempt is true when runDaemon should construct this half's client.
	Attempt bool
}

// Guards groups the run-safety gates (observation deltas, epistemic/intent
// gating, prompt-injection guard).
type Guards struct {
	// ObservationDeltas is AGEZT_OBSERVATION_DELTAS == "on"/"1".
	ObservationDeltas bool
	// EpistemicEscalation is AGEZT_EPISTEMIC_ESCALATION == "on"/"1".
	EpistemicEscalation bool
	// IntentRegretGating is AGEZT_INTENT_REGRET_GATING == "on"/"1".
	IntentRegretGating bool
	// PromptInjectionGuard is the RAW AGEZT_PROMPT_INJECTION_GUARD value;
	// runDaemon parses it with kernelruntime.ParsePromptInjectionMode (any
	// string is valid — unset/unknown means warn mode).
	PromptInjectionGuard string
	// DisableHeuristicBypass is AGEZT_DISABLE_HEURISTIC_BYPASS == "on"/"1".
	DisableHeuristicBypass bool
}

// Tenancy groups the multi-tenant registry settings (ROADMAP P6-MULTI, M14).
// The TENANT_* quotas are parsed ONLY when Multitenant is on — a malformed
// value with tenancy off was ignored inline and still is.
type Tenancy struct {
	// Multitenant is AGEZT_MULTITENANT == "on".
	Multitenant bool
	// DailyCeiling* carry AGEZT_TENANT_DAILY_CEILING (USD). Set distinguishes
	// an explicit value (including 0) from "inherit the primary's ceiling".
	DailyCeilingSet        bool
	DailyCeilingUSD        float64
	DailyCeilingMicrocents int64
	// RatePerMin* carry AGEZT_TENANT_RATE_PER_MIN. Set distinguishes an
	// explicit 0 ("0/min") from unset ("unlimited").
	RatePerMinSet bool
	RatePerMin    int
}

// Lifecycle groups restart/update/disconnect behaviour (M1002, M35, M860).
type Lifecycle struct {
	// Resume is AGEZT_RESUME != "off" (M1002 durable run resume).
	Resume bool
	// ResumeSnapshotMaxBytes is AGEZT_RESUME_SNAPSHOT_MAX_BYTES (0 = package
	// default; malformed/non-positive silently falls back).
	ResumeSnapshotMaxBytes int
	// CancelOnDisconnect is AGEZT_CANCEL_ON_DISCONNECT == "on" (M35).
	CancelOnDisconnect bool
	// UpdateEndpoint is AGEZT_UPDATE_ENDPOINT (raw; empty = not configured).
	UpdateEndpoint string
	// UpdateDrainTimeout is AGEZT_UPDATE_DRAIN_TIMEOUT (default 30s; a
	// malformed value silently keeps the default). Consumed only by the
	// endpoint source — the GitHub source hardcodes 30s, as inline did.
	UpdateDrainTimeout time.Duration
	// UpdateCheckInterval is AGEZT_UPDATE_CHECK_INTERVAL (default 0 =
	// disabled; only a valid positive duration arms it).
	UpdateCheckInterval time.Duration
	// UpdateGitHubOwner / UpdateGitHubRepo are AGEZT_UPDATE_GITHUB_OWNER /
	// _REPO. When the owner is set and the repo empty, the repo defaults to
	// the binary name (as inline did).
	UpdateGitHubOwner string
	UpdateGitHubRepo  string
}

// Misc is the remainder: single strings and switches with no larger cluster.
type Misc struct {
	// SystemPrompt is AGEZT_SYSTEM_PROMPT, raw and untrimmed.
	SystemPrompt string
	// Redact is AGEZT_REDACT != "off" (M15 / SPEC-06 journal scrubbing).
	Redact bool
	// EnvInject is AGEZT_ENV_INJECT != "off" (M609 host-env preamble).
	EnvInject bool
	// CouncilWebSearch is AGEZT_COUNCIL_WEBSEARCH != "off". (The
	// COUNCIL_MEMBERS list stays a live read in its closure.)
	CouncilWebSearch bool
	// ScheduleNotify is AGEZT_SCHEDULE_NOTIFY == "on" (trimmed; M152).
	ScheduleNotify bool
	// WebhookChannels is the AGEZT_WEBHOOK_CHANNELS comma list (trimmed,
	// blanks dropped) — the standing-order briefing allowlist for `webhook`.
	WebhookChannels []string
	// ToolforgeAutoPromote is AGEZT_TOOLFORGE_AUTO_PROMOTE != "off".
	ToolforgeAutoPromote bool
}

// Load parses the daemon's boot configuration from the environment. get is the
// env accessor (nil = os.Getenv; inject a map lookup in tests); warn receives
// the warn-and-degrade messages (nil = discard). The returned error is non-nil
// only for the historically fatal cases, carries the env name, and is emitted
// in the same source order as the inline code so a boot with several bad
// values fails on the same one it always did.
