// SPDX-License-Identifier: MIT

// Daemon config Load function (reads every key from the daemon TOML/env into Config).
// Code extracted from daemonconfig.go during the Day-94 god-file split.
// Public API unchanged.
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
