// SPDX-License-Identifier: MIT

package daemonconfig

import (
	"strings"
	"testing"
	"time"
)

// envOf returns a Load getter backed by a map — the unit-test seam that makes
// the whole boot-config surface testable without touching the process env.
func envOf(m map[string]string) func(string) string {
	return func(name string) string { return m[name] }
}

func load(t *testing.T, env map[string]string) (Config, string) {
	t.Helper()
	var warn strings.Builder
	c, err := Load(envOf(env), &warn)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	return c, warn.String()
}

func loadErr(t *testing.T, env map[string]string) error {
	t.Helper()
	_, err := Load(envOf(env), nil)
	if err == nil {
		t.Fatalf("Load(%v): want error, got nil", env)
	}
	return err
}

func TestLoad_Defaults(t *testing.T) {
	c, warn := load(t, nil)
	if warn != "" {
		t.Errorf("empty env produced warnings: %q", warn)
	}

	// Knowledge: everything on by default except the opt-ins.
	k := c.Knowledge
	if !k.Memory || !k.UserProfile || !k.TasteInject || !k.WorldModel || !k.Skills || !k.Forge {
		t.Errorf("knowledge defaults not all on: %+v", k)
	}
	if k.MemoryDistillMinTools != 6 {
		t.Errorf("MemoryDistillMinTools = %d, want 6", k.MemoryDistillMinTools)
	}
	if k.SkillShadowEval || k.SkillAutoShadow {
		t.Errorf("shadow opt-ins should default off: %+v", k)
	}
	if !k.SkillAutoQuarantine || !k.SkillAutoPromote {
		t.Errorf("skill auto-quarantine/promote should default on: %+v", k)
	}

	// SubAgents: enabled, depth 3, and the M843 derived tree rail of 48.
	sa := c.SubAgents
	if !sa.Enabled || sa.Depth != 3 || sa.Fanout != 0 || sa.SpendCapMicrocents != 0 || sa.MaxTotal != 48 {
		t.Errorf("subagent defaults wrong: %+v", sa)
	}

	// RunLoop / Context / Guards: zero values (kernel defaults).
	if c.RunLoop != (RunLoop{}) {
		t.Errorf("RunLoop defaults not zero: %+v", c.RunLoop)
	}
	if c.Context != (ContextBudget{}) {
		t.Errorf("Context defaults not zero: %+v", c.Context)
	}
	if c.Guards != (Guards{}) {
		t.Errorf("Guards defaults not zero: %+v", c.Guards)
	}

	// Policy.
	if c.Policy.AllowAll || c.Policy.EdictDurable || c.Policy.ApprovalTimeout != 0 || c.Policy.EdictDeny != nil {
		t.Errorf("policy defaults wrong: %+v", c.Policy)
	}

	// Lifecycle: resume on, update drain default 30s, check interval off.
	lc := c.Lifecycle
	if !lc.Resume || lc.ResumeSnapshotMaxBytes != 0 || lc.CancelOnDisconnect {
		t.Errorf("lifecycle defaults wrong: %+v", lc)
	}
	if lc.UpdateDrainTimeout != 30*time.Second || lc.UpdateCheckInterval != 0 {
		t.Errorf("update timing defaults wrong: drain=%s check=%s", lc.UpdateDrainTimeout, lc.UpdateCheckInterval)
	}

	// Tenancy off; Misc defaults.
	if c.Tenancy != (Tenancy{}) {
		t.Errorf("tenancy defaults not zero: %+v", c.Tenancy)
	}
	m := c.Misc
	if !m.Redact || !m.EnvInject || !m.CouncilWebSearch || !m.ToolforgeAutoPromote {
		t.Errorf("misc default-on switches wrong: %+v", m)
	}
	if m.ScheduleNotify || m.SystemPrompt != "" || m.WebhookChannels != nil {
		t.Errorf("misc default-off values wrong: %+v", m)
	}

	// Sidecars all unconfigured.
	if c.Sidecars.Embed.Enabled() || c.Sidecars.Image.Enabled() || c.Sidecars.Rerank.Enabled() ||
		c.Sidecars.STT.Attempt || c.Sidecars.TTS.Attempt {
		t.Errorf("sidecars should be unconfigured by default: %+v", c.Sidecars)
	}
}

func TestLoad_KnowledgeSwitches(t *testing.T) {
	// MEMORY=off also forces the profile off, regardless of USER_PROFILE.
	c, _ := load(t, map[string]string{"AGEZT_MEMORY": "off"})
	if c.Knowledge.Memory || c.Knowledge.UserProfile {
		t.Errorf("MEMORY=off: got memory=%v profile=%v", c.Knowledge.Memory, c.Knowledge.UserProfile)
	}
	c, _ = load(t, map[string]string{"AGEZT_USER_PROFILE": "OFF"}) // EqualFold
	if !c.Knowledge.Memory || c.Knowledge.UserProfile {
		t.Errorf("USER_PROFILE=OFF: got memory=%v profile=%v", c.Knowledge.Memory, c.Knowledge.UserProfile)
	}
	c, _ = load(t, map[string]string{
		"AGEZT_MEMORY_DISTILL_MIN_TOOLS": "12",
		"AGEZT_SKILL_SHADOWEVAL":         "on",
		"AGEZT_SKILL_AUTOQUARANTINE":     "off",
		"AGEZT_SKILL_AUTOSHADOW":         "on",
		"AGEZT_SKILL_AUTOPROMOTE":        "off",
	})
	if c.Knowledge.MemoryDistillMinTools != 12 || !c.Knowledge.SkillShadowEval ||
		c.Knowledge.SkillAutoQuarantine || !c.Knowledge.SkillAutoShadow || c.Knowledge.SkillAutoPromote {
		t.Errorf("knowledge switches wrong: %+v", c.Knowledge)
	}
	// Malformed / non-positive distill threshold silently keeps the default.
	c, warn := load(t, map[string]string{"AGEZT_MEMORY_DISTILL_MIN_TOOLS": "-3"})
	if c.Knowledge.MemoryDistillMinTools != 6 || warn != "" {
		t.Errorf("bad distill threshold: got %d, warn=%q", c.Knowledge.MemoryDistillMinTools, warn)
	}
}

func TestLoad_Policy(t *testing.T) {
	c, _ := load(t, map[string]string{
		"AGEZT_ALLOW_ALL":        "1",
		"AGEZT_EDICT_DENY":       "git push;shell:/etc/shadow",
		"AGEZT_EDICT_DURABLE":    "on",
		"AGEZT_APPROVAL_TIMEOUT": "2m",
	})
	if !c.Policy.AllowAll || !c.Policy.EdictDurable || c.Policy.ApprovalTimeout != 2*time.Minute {
		t.Errorf("policy wrong: %+v", c.Policy)
	}
	if len(c.Policy.EdictDeny) != 2 {
		t.Errorf("EdictDeny = %d rules, want 2", len(c.Policy.EdictDeny))
	}
	// ALLOW_ALL is the exact string "1" — "true" is not honored (inline parity).
	c, _ = load(t, map[string]string{"AGEZT_ALLOW_ALL": "true"})
	if c.Policy.AllowAll {
		t.Error(`ALLOW_ALL="true" should not enable the permissive switch`)
	}
	// Non-positive approval timeout means "use the kernel default".
	c, _ = load(t, map[string]string{"AGEZT_APPROVAL_TIMEOUT": "0s"})
	if c.Policy.ApprovalTimeout != 0 {
		t.Errorf("APPROVAL_TIMEOUT=0s: got %s, want 0", c.Policy.ApprovalTimeout)
	}
	// Malformed deny spec is fatal with the historical env-name prefix.
	err := loadErr(t, map[string]string{"AGEZT_EDICT_DENY": "shell:"})
	if !strings.HasPrefix(err.Error(), "AGEZT_EDICT_DENY: ") {
		t.Errorf("EDICT_DENY error = %q, want AGEZT_EDICT_DENY: prefix", err)
	}
}

func TestLoad_SubAgents(t *testing.T) {
	c, _ := load(t, map[string]string{
		"AGEZT_SUBAGENT":           "off",
		"AGEZT_SUBAGENT_DEPTH":     "5",
		"AGEZT_SUBAGENT_FANOUT":    "2",
		"AGEZT_SUBAGENT_SPEND_CAP": "2.5",
		"AGEZT_SUBAGENT_MAX_TOTAL": "10",
	})
	want := SubAgents{Enabled: false, Depth: 5, Fanout: 2, SpendCapMicrocents: 2_500_000_000, MaxTotal: 10}
	if c.SubAgents != want {
		t.Errorf("subagents = %+v, want %+v", c.SubAgents, want)
	}
	// Depth 1 leaves the tree total unbounded (no M843 derivation).
	c, _ = load(t, map[string]string{"AGEZT_SUBAGENT_DEPTH": "1"})
	if c.SubAgents.MaxTotal != 0 {
		t.Errorf("depth=1: MaxTotal = %d, want 0", c.SubAgents.MaxTotal)
	}
	// Malformed / negative spend cap is fatal with the historical wording.
	for _, v := range []string{"abc", "-1"} {
		err := loadErr(t, map[string]string{"AGEZT_SUBAGENT_SPEND_CAP": v})
		want := `AGEZT_SUBAGENT_SPEND_CAP: want a non-negative USD amount, got "` + v + `"`
		if err.Error() != want {
			t.Errorf("spend cap %q: error = %q, want %q", v, err, want)
		}
	}
}

func TestLoad_ContextBudget_WarnAndDegrade(t *testing.T) {
	c, warn := load(t, map[string]string{
		"AGEZT_ARTIFACT_THRESHOLD":    "-5",
		"AGEZT_CONTEXT_BUDGET":        "abc",
		"AGEZT_CONTEXT_PROTECT_FIRST": "-1",
	})
	if c.Context.ArtifactThreshold != 0 || c.Context.Budget != 0 || c.Context.BudgetAuto || c.Context.ProtectFirst != 0 {
		t.Errorf("bad values should keep defaults: %+v", c.Context)
	}
	for _, wantLine := range []string{
		`agezt: AGEZT_ARTIFACT_THRESHOLD: want a positive byte count, got "-5" (using default)`,
		`agezt: AGEZT_CONTEXT_BUDGET: want a positive char count or "auto", got "abc" (ignored)`,
		`agezt: AGEZT_CONTEXT_PROTECT_FIRST: want a non-negative count, got "-1" (ignored)`,
	} {
		if !strings.Contains(warn, wantLine) {
			t.Errorf("warning output missing %q; got:\n%s", wantLine, warn)
		}
	}

	c, warn = load(t, map[string]string{
		"AGEZT_ARTIFACT_THRESHOLD":    "65536",
		"AGEZT_CONTEXT_BUDGET":        "AUTO", // EqualFold
		"AGEZT_CONTEXT_PROTECT_FIRST": "4",
		"AGEZT_CONTEXT_SUMMARIZE":     "1",
	})
	if warn != "" {
		t.Errorf("happy path warned: %q", warn)
	}
	want := ContextBudget{ArtifactThreshold: 65536, BudgetAuto: true, ProtectFirst: 4, Summarize: true}
	if c.Context != want {
		t.Errorf("context = %+v, want %+v", c.Context, want)
	}
	c, _ = load(t, map[string]string{"AGEZT_CONTEXT_BUDGET": "120000"})
	if c.Context.Budget != 120000 || c.Context.BudgetAuto {
		t.Errorf("numeric budget wrong: %+v", c.Context)
	}
}

func TestLoad_Guards(t *testing.T) {
	c, _ := load(t, map[string]string{
		"AGEZT_OBSERVATION_DELTAS":       "ON", // EqualFold + trims
		"AGEZT_EPISTEMIC_ESCALATION":     " 1 ",
		"AGEZT_INTENT_REGRET_GATING":     "on",
		"AGEZT_PROMPT_INJECTION_GUARD":   "block",
		"AGEZT_DISABLE_HEURISTIC_BYPASS": "1",
	})
	want := Guards{
		ObservationDeltas: true, EpistemicEscalation: true, IntentRegretGating: true,
		PromptInjectionGuard: "block", DisableHeuristicBypass: true,
	}
	if c.Guards != want {
		t.Errorf("guards = %+v, want %+v", c.Guards, want)
	}
	// Anything other than on/1 stays off.
	c, _ = load(t, map[string]string{"AGEZT_OBSERVATION_DELTAS": "yes"})
	if c.Guards.ObservationDeltas {
		t.Error(`OBSERVATION_DELTAS="yes" should stay off`)
	}
}

func TestLoad_Sidecars(t *testing.T) {
	// URL without model: warn + disabled, for each URL/model cluster.
	for _, tc := range []struct{ urlVar, wantMsg string }{
		{"AGEZT_EMBED_URL", "AGEZT_EMBED_URL is set but AGEZT_EMBED_MODEL is empty — provider embeddings disabled"},
		{"AGEZT_IMAGE_URL", "AGEZT_IMAGE_URL is set but AGEZT_IMAGE_MODEL is empty — image generation disabled"},
		{"AGEZT_RERANK_URL", "AGEZT_RERANK_URL is set but AGEZT_RERANK_MODEL is empty — reranking disabled"},
		{"AGEZT_STT_URL", "AGEZT_STT_URL is set but AGEZT_STT_MODEL is empty — transcription disabled"},
		{"AGEZT_TTS_URL", "AGEZT_TTS_URL is set but AGEZT_TTS_MODEL is empty — synthesis disabled"},
	} {
		c, warn := load(t, map[string]string{tc.urlVar: "http://localhost:9999"})
		if !strings.Contains(warn, "agezt: "+tc.wantMsg) {
			t.Errorf("%s without model: warn = %q, want %q", tc.urlVar, warn, tc.wantMsg)
		}
		if c.Sidecars.Embed.Enabled() || c.Sidecars.Image.Enabled() || c.Sidecars.Rerank.Enabled() ||
			c.Sidecars.STT.Attempt || c.Sidecars.TTS.Attempt {
			t.Errorf("%s without model should leave every sidecar disabled", tc.urlVar)
		}
	}

	// Happy path: URL + model (values trimmed).
	c, warn := load(t, map[string]string{
		"AGEZT_EMBED_URL":   " http://localhost:11434 ",
		"AGEZT_EMBED_MODEL": "nomic-embed-text",
		"AGEZT_EMBED_KEY":   " k1 ",
		"AGEZT_STT_URL":     "http://localhost:8001",
		"AGEZT_STT_MODEL":   "whisper-1",
		"AGEZT_TTS_URL":     "http://localhost:8002",
		"AGEZT_TTS_MODEL":   "kokoro",
		"AGEZT_TTS_VOICE":   "af_bella",
	})
	if warn != "" {
		t.Errorf("configured sidecars warned: %q", warn)
	}
	if e := c.Sidecars.Embed; !e.Enabled() || e.URL != "http://localhost:11434" || e.Key != "k1" {
		t.Errorf("embed = %+v", e)
	}
	if s := c.Sidecars.STT; !s.Attempt || s.Model != "whisper-1" {
		t.Errorf("stt = %+v", s)
	}
	if tts := c.Sidecars.TTS; !tts.Attempt || tts.Voice != "af_bella" {
		t.Errorf("tts = %+v", tts)
	}

	// Native voice providers need neither URL nor model.
	c, warn = load(t, map[string]string{"AGEZT_STT_PROVIDER": "ElevenLabs", "AGEZT_TTS_PROVIDER": "deepgram"})
	if warn != "" {
		t.Errorf("native providers warned: %q", warn)
	}
	if !c.Sidecars.STT.Attempt || !c.Sidecars.TTS.Attempt {
		t.Errorf("native providers should attempt: stt=%+v tts=%+v", c.Sidecars.STT, c.Sidecars.TTS)
	}
}

func TestLoad_RunLoop(t *testing.T) {
	c, _ := load(t, map[string]string{
		"AGEZT_RUN_TIMEOUT":        "5m",
		"AGEZT_MAX_ITER":           "50",
		"AGEZT_MAX_AUTO_CONTINUE":  "-1",
		"AGEZT_AUTO_CONTINUE_WAIT": "30s",
		"AGEZT_PARALLEL_TOOLS":     "4",
		"AGEZT_TOOL_DISCOVERY_MAX": "0",
		"AGEZT_TOOL_TIMEOUT":       "45s",
	})
	want := RunLoop{
		RunTimeout: 5 * time.Minute, MaxIter: 50,
		MaxAutoContinue: -1, MaxAutoContinueSet: true,
		AutoContinueWait: 30 * time.Second, MaxParallelTools: 4,
		ToolDiscoveryMax: 0, ToolTimeout: 45 * time.Second,
	}
	if c.RunLoop != want {
		t.Errorf("runloop = %+v, want %+v", c.RunLoop, want)
	}
	// A valid but non-positive run/tool timeout means "off", not an error.
	c, _ = load(t, map[string]string{"AGEZT_RUN_TIMEOUT": "-5m", "AGEZT_TOOL_TIMEOUT": "0s"})
	if c.RunLoop.RunTimeout != 0 || c.RunLoop.ToolTimeout != 0 {
		t.Errorf("non-positive timeouts should disarm: %+v", c.RunLoop)
	}
	// An explicit auto-continue of 0 is Set (banner shows the default label).
	c, _ = load(t, map[string]string{"AGEZT_MAX_AUTO_CONTINUE": "0"})
	if !c.RunLoop.MaxAutoContinueSet || c.RunLoop.MaxAutoContinue != 0 {
		t.Errorf("MAX_AUTO_CONTINUE=0: %+v", c.RunLoop)
	}

	// Fatal cases carry the historical wording, env name first.
	for _, tc := range []struct{ env, val, want string }{
		{"AGEZT_RUN_TIMEOUT", "xyz", `AGEZT_RUN_TIMEOUT: want a Go duration (e.g. 90s, 5m), got "xyz"`},
		{"AGEZT_MAX_ITER", "0", `AGEZT_MAX_ITER: want a positive integer, got "0"`},
		{"AGEZT_MAX_ITER", "abc", `AGEZT_MAX_ITER: want a positive integer, got "abc"`},
		{"AGEZT_MAX_AUTO_CONTINUE", "many", `AGEZT_MAX_AUTO_CONTINUE: want an integer, got "many"`},
		{"AGEZT_AUTO_CONTINUE_WAIT", "-1s", `AGEZT_AUTO_CONTINUE_WAIT: want a non-negative duration, got "-1s"`},
		{"AGEZT_PARALLEL_TOOLS", "0", `AGEZT_PARALLEL_TOOLS: want a positive integer, got "0"`},
		{"AGEZT_TOOL_DISCOVERY_MAX", "-1", `AGEZT_TOOL_DISCOVERY_MAX: want a non-negative integer, got "-1"`},
		{"AGEZT_TOOL_TIMEOUT", "soon", `AGEZT_TOOL_TIMEOUT: want a Go duration (e.g. 30s, 2m), got "soon"`},
		{"AGEZT_APPROVAL_TIMEOUT", "later", `AGEZT_APPROVAL_TIMEOUT: want a Go duration (e.g. 2m, 30s), got "later"`},
	} {
		err := loadErr(t, map[string]string{tc.env: tc.val})
		if err.Error() != tc.want {
			t.Errorf("%s=%s: error = %q, want %q", tc.env, tc.val, err, tc.want)
		}
	}
}

func TestLoad_Tenancy(t *testing.T) {
	// Quotas are ignored (even malformed) while multi-tenancy is off.
	c, warn := load(t, map[string]string{
		"AGEZT_TENANT_DAILY_CEILING": "not-a-number",
		"AGEZT_TENANT_RATE_PER_MIN":  "-9",
	})
	if c.Tenancy != (Tenancy{}) || warn != "" {
		t.Errorf("quotas with tenancy off should be ignored: %+v warn=%q", c.Tenancy, warn)
	}

	c, _ = load(t, map[string]string{
		"AGEZT_MULTITENANT":          "ON", // EqualFold
		"AGEZT_TENANT_DAILY_CEILING": "1.5",
		"AGEZT_TENANT_RATE_PER_MIN":  "0",
	})
	want := Tenancy{
		Multitenant:     true,
		DailyCeilingSet: true, DailyCeilingUSD: 1.5, DailyCeilingMicrocents: 1_500_000_000,
		RatePerMinSet: true, RatePerMin: 0,
	}
	if c.Tenancy != want {
		t.Errorf("tenancy = %+v, want %+v", c.Tenancy, want)
	}

	// With tenancy on, malformed quotas are fatal with the historical wording.
	err := loadErr(t, map[string]string{"AGEZT_MULTITENANT": "on", "AGEZT_TENANT_DAILY_CEILING": "-1"})
	if err.Error() != `AGEZT_TENANT_DAILY_CEILING: want a non-negative USD amount, got "-1"` {
		t.Errorf("ceiling error = %q", err)
	}
	err = loadErr(t, map[string]string{"AGEZT_MULTITENANT": "on", "AGEZT_TENANT_RATE_PER_MIN": "fast"})
	if err.Error() != `AGEZT_TENANT_RATE_PER_MIN: want a non-negative integer, got "fast"` {
		t.Errorf("rate error = %q", err)
	}
}

func TestLoad_Lifecycle(t *testing.T) {
	c, _ := load(t, map[string]string{
		"AGEZT_RESUME":                    "off",
		"AGEZT_RESUME_SNAPSHOT_MAX_BYTES": "1048576",
		"AGEZT_CANCEL_ON_DISCONNECT":      "on",
		"AGEZT_UPDATE_ENDPOINT":           "https://updates.example/check",
		"AGEZT_UPDATE_DRAIN_TIMEOUT":      "5s",
		"AGEZT_UPDATE_CHECK_INTERVAL":     "1h",
	})
	lc := c.Lifecycle
	if lc.Resume || lc.ResumeSnapshotMaxBytes != 1048576 || !lc.CancelOnDisconnect {
		t.Errorf("lifecycle switches wrong: %+v", lc)
	}
	if lc.UpdateEndpoint != "https://updates.example/check" ||
		lc.UpdateDrainTimeout != 5*time.Second || lc.UpdateCheckInterval != time.Hour {
		t.Errorf("update settings wrong: %+v", lc)
	}
	// Malformed drain timeout silently keeps 30s; non-positive interval stays off.
	c, warn := load(t, map[string]string{
		"AGEZT_UPDATE_DRAIN_TIMEOUT":  "whenever",
		"AGEZT_UPDATE_CHECK_INTERVAL": "-1h",
	})
	if warn != "" || c.Lifecycle.UpdateDrainTimeout != 30*time.Second || c.Lifecycle.UpdateCheckInterval != 0 {
		t.Errorf("update fallback wrong: %+v warn=%q", c.Lifecycle, warn)
	}
	// GitHub source: repo defaults to the binary name when only the owner is set.
	c, _ = load(t, map[string]string{"AGEZT_UPDATE_GITHUB_OWNER": "agezt"})
	if c.Lifecycle.UpdateGitHubRepo != "agezt" {
		t.Errorf("github repo default = %q, want binary name", c.Lifecycle.UpdateGitHubRepo)
	}
	c, _ = load(t, map[string]string{"AGEZT_UPDATE_GITHUB_OWNER": "agezt", "AGEZT_UPDATE_GITHUB_REPO": "kernel"})
	if c.Lifecycle.UpdateGitHubRepo != "kernel" {
		t.Errorf("explicit github repo = %q", c.Lifecycle.UpdateGitHubRepo)
	}
}

func TestLoad_Misc(t *testing.T) {
	c, _ := load(t, map[string]string{
		"AGEZT_SYSTEM_PROMPT":          "  keep my spaces  ",
		"AGEZT_REDACT":                 "off",
		"AGEZT_ENV_INJECT":             "off",
		"AGEZT_COUNCIL_WEBSEARCH":      "off",
		"AGEZT_SCHEDULE_NOTIFY":        " on ",
		"AGEZT_WEBHOOK_CHANNELS":       "a, b,,c",
		"AGEZT_TOOLFORGE_AUTO_PROMOTE": "off",
	})
	m := c.Misc
	if m.SystemPrompt != "  keep my spaces  " {
		t.Errorf("SystemPrompt must stay untrimmed, got %q", m.SystemPrompt)
	}
	if m.Redact || m.EnvInject || m.CouncilWebSearch || m.ToolforgeAutoPromote {
		t.Errorf("misc off-switches wrong: %+v", m)
	}
	if !m.ScheduleNotify {
		t.Error("SCHEDULE_NOTIFY should trim before comparing to \"on\"")
	}
	if got := strings.Join(m.WebhookChannels, "|"); got != "a|b|c" {
		t.Errorf("WebhookChannels = %q, want a|b|c", got)
	}
}

// TestLoad_FatalOrder pins the fail-fast order: with several bad values set,
// Load reports the same first one the old inline code exited on (EDICT_DENY
// was parsed before the run-loop caps).
func TestLoad_FatalOrder(t *testing.T) {
	err := loadErr(t, map[string]string{
		"AGEZT_EDICT_DENY":  "shell:",
		"AGEZT_RUN_TIMEOUT": "xyz",
	})
	if !strings.HasPrefix(err.Error(), "AGEZT_EDICT_DENY: ") {
		t.Errorf("first fatal should be EDICT_DENY, got %q", err)
	}
	err = loadErr(t, map[string]string{
		"AGEZT_SUBAGENT_SPEND_CAP": "-2",
		"AGEZT_MAX_ITER":           "abc",
	})
	if !strings.HasPrefix(err.Error(), "AGEZT_SUBAGENT_SPEND_CAP: ") {
		t.Errorf("spend cap should fail before MAX_ITER, got %q", err)
	}
}

// TestLoad_NilAccessorAndWriter exercises the documented nil conveniences
// (os.Getenv + discarded warnings) without asserting on ambient env values.
func TestLoad_NilAccessorAndWriter(t *testing.T) {
	if _, err := Load(nil, nil); err != nil {
		// The ambient environment could legitimately carry a malformed value;
		// only fail on the classes of error this test can't explain.
		t.Logf("Load with ambient env returned: %v (tolerated)", err)
	}
}
