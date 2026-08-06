// SPDX-License-Identifier: MIT

// Package daemonconfig parses the daemon's AGEZT_* boot environment into one
// typed Config (Phase 2.5). It replaces the ~450 lines of inline env parsing
// that lived in runDaemon, so every parse shape, default, warning string, and
// fatal-vs-warn decision is unit-testable without booting a daemon.
//
// Load MUST be called after injectConfig has bridged the Config Center store +
// vault into the process environment (the reads here see injected values, same
// as the inline code they replace). Reads that must NOT go through Load stay in
// runDaemon:
//
//   - AGEZT_FORCE_START — read before injectConfig runs (single-instance guard).
//   - AGEZT_COUNCIL_MEMBERS, AGEZT_DRAIN_TIMEOUT, and the tenant lazy-open's
//     AGEZT_EDICT_DURABLE — read live at call time (the Config Center
//     live-applies edits via os.Setenv, so a boot-time capture would regress
//     restart-free reconfiguration).
//   - channel-manifest RequiredEnv/AllowlistEnv names — dynamic, not AGEZT_*
//     statics.
//   - AGEZT_WEB_ADDR / AGEZT_REST_ADDR / AGEZT_API_ADDR — owned by their
//     buildWebUI/buildRESTAPI/buildOpenAIAPI helpers.
//
// Semantics contract: Load returns an error ONLY for the values that were a
// hard startup failure in the inline code (malformed durations/integers on the
// run-loop caps, malformed USD amounts, malformed deny rules, malformed tenant
// quotas when multi-tenancy is on). Everything else warns to the writer with
// the exact historical message and falls back to its default. Error strings
// carry the env name ("AGEZT_X: want ..."), so runDaemon's
// `fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)` reproduces the
// historical output byte-for-byte.
package daemonconfig

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/plugins/providers/voice"
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
func Load(get func(string) string, warn io.Writer) (Config, error) {
	if get == nil {
		get = os.Getenv
	}
	if warn == nil {
		warn = io.Discard
	}
	var c Config

	// --- Policy: operator deny rules + master permissive switch (M17/M611) ---
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "EDICT_DENY")); spec != "" {
		extra, derr := edict.ParseDenyRules(spec)
		if derr != nil {
			return Config{}, fmt.Errorf("%sEDICT_DENY: %w", brand.EnvPrefix, derr)
		}
		c.Policy.EdictDeny = extra
	}
	c.Policy.AllowAll = get(brand.EnvPrefix+"ALLOW_ALL") == "1"

	// --- Knowledge: memory / taste / world / skills switches ---
	c.Knowledge.Memory = !strings.EqualFold(get(brand.EnvPrefix+"MEMORY"), "off")
	c.Knowledge.MemoryDistillMinTools = 6
	if v := strings.TrimSpace(get(brand.EnvPrefix + "MEMORY_DISTILL_MIN_TOOLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Knowledge.MemoryDistillMinTools = n
		}
	}
	c.Knowledge.UserProfile = c.Knowledge.Memory && !strings.EqualFold(get(brand.EnvPrefix+"USER_PROFILE"), "off")
	c.Knowledge.TasteInject = !strings.EqualFold(get(brand.EnvPrefix+"TASTE_INJECT"), "off")
	c.Knowledge.WorldModel = !strings.EqualFold(get(brand.EnvPrefix+"WORLDMODEL"), "off")
	c.Knowledge.Skills = !strings.EqualFold(get(brand.EnvPrefix+"SKILLS"), "off")
	c.Knowledge.Forge = !strings.EqualFold(get(brand.EnvPrefix+"FORGE"), "off")
	c.Misc.EnvInject = !strings.EqualFold(get(brand.EnvPrefix+"ENV_INJECT"), "off")

	// --- Lifecycle: durable resume (M1002) ---
	c.Lifecycle.Resume = !strings.EqualFold(get(brand.EnvPrefix+"RESUME"), "off")
	if v := strings.TrimSpace(get(brand.EnvPrefix + "RESUME_SNAPSHOT_MAX_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Lifecycle.ResumeSnapshotMaxBytes = n
		}
	}

	// --- SubAgents: delegation rails ---
	c.SubAgents.Enabled = !strings.EqualFold(get(brand.EnvPrefix+"SUBAGENT"), "off")
	c.SubAgents.Depth = 3
	if v := strings.TrimSpace(get(brand.EnvPrefix + "SUBAGENT_DEPTH")); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			c.SubAgents.Depth = d
		}
	}
	if v := strings.TrimSpace(get(brand.EnvPrefix + "SUBAGENT_FANOUT")); v != "" {
		if f, err := strconv.Atoi(v); err == nil && f > 0 {
			c.SubAgents.Fanout = f
		}
	}
	if v := strings.TrimSpace(get(brand.EnvPrefix + "SUBAGENT_SPEND_CAP")); v != "" {
		usd, perr := strconv.ParseFloat(v, 64)
		if perr != nil || usd < 0 {
			return Config{}, fmt.Errorf("%sSUBAGENT_SPEND_CAP: want a non-negative USD amount, got %q", brand.EnvPrefix, v)
		}
		c.SubAgents.SpendCapMicrocents = int64(usd * 1e9)
	}
	if v := strings.TrimSpace(get(brand.EnvPrefix + "SUBAGENT_MAX_TOTAL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.SubAgents.MaxTotal = n
		}
	}
	// M843: deep delegation gets a finite tree rail when the operator set none.
	if c.SubAgents.MaxTotal == 0 && c.SubAgents.Depth > 1 {
		c.SubAgents.MaxTotal = 48
	}

	// --- ContextBudget: artifact offload + context caps (warn + default) ---
	if v := get(brand.EnvPrefix + "ARTIFACT_THRESHOLD"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			c.Context.ArtifactThreshold = n
		} else {
			fmt.Fprintf(warn, "%s: %sARTIFACT_THRESHOLD: want a positive byte count, got %q (using default)\n", brand.Binary, brand.EnvPrefix, v)
		}
	}
	if v := get(brand.EnvPrefix + "CONTEXT_BUDGET"); v != "" {
		if strings.EqualFold(v, "auto") {
			c.Context.BudgetAuto = true // derive from the model's catalog context window
		} else if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			c.Context.Budget = n
		} else {
			fmt.Fprintf(warn, "%s: %sCONTEXT_BUDGET: want a positive char count or \"auto\", got %q (ignored)\n", brand.Binary, brand.EnvPrefix, v)
		}
	}
	if v := get(brand.EnvPrefix + "CONTEXT_PROTECT_FIRST"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n >= 0 {
			c.Context.ProtectFirst = n
		} else {
			fmt.Fprintf(warn, "%s: %sCONTEXT_PROTECT_FIRST: want a non-negative count, got %q (ignored)\n", brand.Binary, brand.EnvPrefix, v)
		}
	}
	c.Context.Summarize = get(brand.EnvPrefix+"CONTEXT_SUMMARIZE") == "1"

	// --- Guards ---
	obsRaw := strings.TrimSpace(get(brand.EnvPrefix + "OBSERVATION_DELTAS"))
	c.Guards.ObservationDeltas = strings.EqualFold(obsRaw, "on") || obsRaw == "1"
	epiRaw := strings.TrimSpace(get(brand.EnvPrefix + "EPISTEMIC_ESCALATION"))
	c.Guards.EpistemicEscalation = strings.EqualFold(epiRaw, "on") || epiRaw == "1"
	intRaw := strings.TrimSpace(get(brand.EnvPrefix + "INTENT_REGRET_GATING"))
	c.Guards.IntentRegretGating = strings.EqualFold(intRaw, "on") || intRaw == "1"
	c.Guards.PromptInjectionGuard = get(brand.EnvPrefix + "PROMPT_INJECTION_GUARD")
	c.Misc.ToolforgeAutoPromote = !strings.EqualFold(get(brand.EnvPrefix+"TOOLFORGE_AUTO_PROMOTE"), "off")
	dhbRaw := strings.TrimSpace(get(brand.EnvPrefix + "DISABLE_HEURISTIC_BYPASS"))
	c.Guards.DisableHeuristicBypass = strings.EqualFold(dhbRaw, "on") || dhbRaw == "1"
	c.Knowledge.SkillShadowEval = strings.EqualFold(get(brand.EnvPrefix+"SKILL_SHADOWEVAL"), "on")

	// --- Sidecars: embeddings / voice / image / rerank (warn + degrade) ---
	c.Sidecars.Embed.URL = strings.TrimSpace(get(brand.EnvPrefix + "EMBED_URL"))
	c.Sidecars.Embed.Model = strings.TrimSpace(get(brand.EnvPrefix + "EMBED_MODEL"))
	c.Sidecars.Embed.Key = strings.TrimSpace(get(brand.EnvPrefix + "EMBED_KEY"))
	if c.Sidecars.Embed.URL != "" && c.Sidecars.Embed.Model == "" {
		fmt.Fprintf(warn, "%s: %sEMBED_URL is set but %sEMBED_MODEL is empty — provider embeddings disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
	}

	c.Sidecars.STT.Provider = strings.TrimSpace(get(brand.EnvPrefix + "STT_PROVIDER"))
	c.Sidecars.STT.URL = strings.TrimSpace(get(brand.EnvPrefix + "STT_URL"))
	c.Sidecars.STT.Model = strings.TrimSpace(get(brand.EnvPrefix + "STT_MODEL"))
	c.Sidecars.STT.Key = strings.TrimSpace(get(brand.EnvPrefix + "STT_KEY"))
	if native := voiceProviderIsNative(c.Sidecars.STT.Provider); c.Sidecars.STT.URL != "" || native {
		if c.Sidecars.STT.Model == "" && !native {
			fmt.Fprintf(warn, "%s: %sSTT_URL is set but %sSTT_MODEL is empty — transcription disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			c.Sidecars.STT.Attempt = true
		}
	}
	c.Sidecars.TTS.Provider = strings.TrimSpace(get(brand.EnvPrefix + "TTS_PROVIDER"))
	c.Sidecars.TTS.URL = strings.TrimSpace(get(brand.EnvPrefix + "TTS_URL"))
	c.Sidecars.TTS.Model = strings.TrimSpace(get(brand.EnvPrefix + "TTS_MODEL"))
	c.Sidecars.TTS.Key = strings.TrimSpace(get(brand.EnvPrefix + "TTS_KEY"))
	c.Sidecars.TTS.Voice = strings.TrimSpace(get(brand.EnvPrefix + "TTS_VOICE"))
	if native := voiceProviderIsNative(c.Sidecars.TTS.Provider); c.Sidecars.TTS.URL != "" || native {
		if c.Sidecars.TTS.Model == "" && !native {
			fmt.Fprintf(warn, "%s: %sTTS_URL is set but %sTTS_MODEL is empty — synthesis disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			c.Sidecars.TTS.Attempt = true
		}
	}

	c.Sidecars.Image.URL = strings.TrimSpace(get(brand.EnvPrefix + "IMAGE_URL"))
	c.Sidecars.Image.Model = strings.TrimSpace(get(brand.EnvPrefix + "IMAGE_MODEL"))
	c.Sidecars.Image.Key = strings.TrimSpace(get(brand.EnvPrefix + "IMAGE_KEY"))
	if c.Sidecars.Image.URL != "" && c.Sidecars.Image.Model == "" {
		fmt.Fprintf(warn, "%s: %sIMAGE_URL is set but %sIMAGE_MODEL is empty — image generation disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
	}
	c.Sidecars.Rerank.URL = strings.TrimSpace(get(brand.EnvPrefix + "RERANK_URL"))
	c.Sidecars.Rerank.Model = strings.TrimSpace(get(brand.EnvPrefix + "RERANK_MODEL"))
	c.Sidecars.Rerank.Key = strings.TrimSpace(get(brand.EnvPrefix + "RERANK_KEY"))
	if c.Sidecars.Rerank.URL != "" && c.Sidecars.Rerank.Model == "" {
		fmt.Fprintf(warn, "%s: %sRERANK_URL is set but %sRERANK_MODEL is empty — reranking disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
	}

	// --- Misc: system prompt (raw, untrimmed — inline never trimmed it) ---
	c.Misc.SystemPrompt = get(brand.EnvPrefix + "SYSTEM_PROMPT")

	// --- RunLoop: per-run caps (every malformed value is fatal) ---
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "RUN_TIMEOUT")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil {
			return Config{}, fmt.Errorf("%sRUN_TIMEOUT: want a Go duration (e.g. 90s, 5m), got %q", brand.EnvPrefix, spec)
		}
		if d > 0 {
			c.RunLoop.RunTimeout = d
		}
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "MAX_ITER")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n <= 0 {
			return Config{}, fmt.Errorf("%sMAX_ITER: want a positive integer, got %q", brand.EnvPrefix, spec)
		}
		c.RunLoop.MaxIter = n
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "MAX_AUTO_CONTINUE")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil {
			return Config{}, fmt.Errorf("%sMAX_AUTO_CONTINUE: want an integer, got %q", brand.EnvPrefix, spec)
		}
		c.RunLoop.MaxAutoContinue = n
		c.RunLoop.MaxAutoContinueSet = true
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "AUTO_CONTINUE_WAIT")); spec != "" {
		d, perr := time.ParseDuration(spec)
		if perr != nil || d < 0 {
			return Config{}, fmt.Errorf("%sAUTO_CONTINUE_WAIT: want a non-negative duration, got %q", brand.EnvPrefix, spec)
		}
		c.RunLoop.AutoContinueWait = d
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "PARALLEL_TOOLS")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n <= 0 {
			return Config{}, fmt.Errorf("%sPARALLEL_TOOLS: want a positive integer, got %q", brand.EnvPrefix, spec)
		}
		c.RunLoop.MaxParallelTools = n
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TOOL_DISCOVERY_MAX")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n < 0 {
			return Config{}, fmt.Errorf("%sTOOL_DISCOVERY_MAX: want a non-negative integer, got %q", brand.EnvPrefix, spec)
		}
		c.RunLoop.ToolDiscoveryMax = n
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TOOL_TIMEOUT")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil {
			return Config{}, fmt.Errorf("%sTOOL_TIMEOUT: want a Go duration (e.g. 30s, 2m), got %q", brand.EnvPrefix, spec)
		}
		if d > 0 {
			c.RunLoop.ToolTimeout = d
		}
	}
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "APPROVAL_TIMEOUT")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil {
			return Config{}, fmt.Errorf("%sAPPROVAL_TIMEOUT: want a Go duration (e.g. 2m, 30s), got %q", brand.EnvPrefix, spec)
		}
		if d > 0 {
			c.Policy.ApprovalTimeout = d
		}
	}

	// --- Misc switches ---
	c.Misc.Redact = !strings.EqualFold(get(brand.EnvPrefix+"REDACT"), "off")
	c.Misc.CouncilWebSearch = !strings.EqualFold(get(brand.EnvPrefix+"COUNCIL_WEBSEARCH"), "off")
	c.Knowledge.SkillAutoQuarantine = !strings.EqualFold(get(brand.EnvPrefix+"SKILL_AUTOQUARANTINE"), "off")
	c.Knowledge.SkillAutoShadow = strings.EqualFold(get(brand.EnvPrefix+"SKILL_AUTOSHADOW"), "on")
	c.Knowledge.SkillAutoPromote = !strings.EqualFold(get(brand.EnvPrefix+"SKILL_AUTOPROMOTE"), "off")
	c.Policy.EdictDurable = strings.EqualFold(get(brand.EnvPrefix+"EDICT_DURABLE"), "on")

	// --- Lifecycle: disconnect + self-update (M35, M860) ---
	c.Lifecycle.CancelOnDisconnect = strings.EqualFold(get(brand.EnvPrefix+"CANCEL_ON_DISCONNECT"), "on")
	c.Lifecycle.UpdateEndpoint = get(brand.EnvPrefix + "UPDATE_ENDPOINT")
	c.Lifecycle.UpdateDrainTimeout = 30 * time.Second
	if t := get(brand.EnvPrefix + "UPDATE_DRAIN_TIMEOUT"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			c.Lifecycle.UpdateDrainTimeout = d
		}
	}
	if t := get(brand.EnvPrefix + "UPDATE_CHECK_INTERVAL"); t != "" {
		if d, err := time.ParseDuration(t); err == nil && d > 0 {
			c.Lifecycle.UpdateCheckInterval = d
		}
	}
	c.Lifecycle.UpdateGitHubOwner = get(brand.EnvPrefix + "UPDATE_GITHUB_OWNER")
	c.Lifecycle.UpdateGitHubRepo = get(brand.EnvPrefix + "UPDATE_GITHUB_REPO")
	if c.Lifecycle.UpdateGitHubOwner != "" && c.Lifecycle.UpdateGitHubRepo == "" {
		c.Lifecycle.UpdateGitHubRepo = brand.Binary // default repo to the binary name
	}

	// --- Tenancy (quotas parsed only when multi-tenancy is on) ---
	c.Tenancy.Multitenant = strings.EqualFold(get(brand.EnvPrefix+"MULTITENANT"), "on")
	if c.Tenancy.Multitenant {
		if spec := strings.TrimSpace(get(brand.EnvPrefix + "TENANT_DAILY_CEILING")); spec != "" {
			usd, perr := strconv.ParseFloat(spec, 64)
			if perr != nil || usd < 0 {
				return Config{}, fmt.Errorf("%sTENANT_DAILY_CEILING: want a non-negative USD amount, got %q", brand.EnvPrefix, spec)
			}
			c.Tenancy.DailyCeilingSet = true
			c.Tenancy.DailyCeilingUSD = usd
			c.Tenancy.DailyCeilingMicrocents = int64(usd * 1e9)
		}
		if spec := strings.TrimSpace(get(brand.EnvPrefix + "TENANT_RATE_PER_MIN")); spec != "" {
			n, perr := strconv.Atoi(spec)
			if perr != nil || n < 0 {
				return Config{}, fmt.Errorf("%sTENANT_RATE_PER_MIN: want a non-negative integer, got %q", brand.EnvPrefix, spec)
			}
			c.Tenancy.RatePerMinSet = true
			c.Tenancy.RatePerMin = n
		}
	}

	// --- Misc: scheduled-run delivery + webhook briefing allowlist ---
	c.Misc.ScheduleNotify = strings.TrimSpace(get(brand.EnvPrefix+"SCHEDULE_NOTIFY")) == "on"
	c.Misc.WebhookChannels = splitNonEmpty(get(brand.EnvPrefix + "WEBHOOK_CHANNELS"))

	return c, nil
}

// voiceProviderIsNative mirrors runDaemon's helper: a native (non-OpenAI-
// compatible) STT/TTS backend supplies its own default base URL, so a URL
// isn't required to enable that half.
func voiceProviderIsNative(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case voice.ProviderElevenLabs, voice.ProviderDeepgram, voice.ProviderCartesia:
		return true
	default:
		return false
	}
}

// splitNonEmpty splits a comma list, trimming and dropping blanks (mirrors
// cmd/agezt's helper).
func splitNonEmpty(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
