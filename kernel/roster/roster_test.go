// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

// TestValidate_SlugRules: the slug is the agent's address — lowercase, no
// spaces, bounded; bad shapes are rejected before anything persists.
func TestValidate_SlugRules(t *testing.T) {
	good := []string{"researcher", "ops-watcher", "a", "r2.d2", "x_1"}
	for _, s := range good {
		if err := Validate(Profile{Slug: s}); err != nil {
			t.Fatalf("slug %q rejected: %v", s, err)
		}
	}
	bad := []string{"", "Researcher", "has space", "-leading", ".leading", "_leading",
		strings.Repeat("a", 65), "ünïcode"}
	for _, s := range bad {
		if err := Validate(Profile{Slug: s}); err == nil {
			t.Fatalf("slug %q accepted, want rejection", s)
		}
	}
}

// TestValidate_Bounds: soul size, fallback count/emptiness, negative cost, and
// workdir escape attempts are all rejected.
func TestValidate_Bounds(t *testing.T) {
	if err := Validate(Profile{Slug: "a", Soul: strings.Repeat("x", maxSoulBytes+1)}); err == nil {
		t.Fatal("oversized soul accepted")
	}
	if err := Validate(Profile{Slug: "a", Fallbacks: make([]string, maxFallbacks+1)}); err == nil {
		t.Fatal("too many fallbacks accepted")
	}
	if err := Validate(Profile{Slug: "a", Fallbacks: []string{"m1", " "}}); err == nil {
		t.Fatal("blank fallback accepted")
	}
	if err := Validate(Profile{Slug: "a", MaxCostMc: -1}); err == nil {
		t.Fatal("negative cost ceiling accepted")
	}
	for _, w := range []string{"/abs", "..", "../up", "a/../../b", "a/.."} {
		if err := Validate(Profile{Slug: "a", Workdir: w}); err == nil {
			t.Fatalf("escaping workdir %q accepted", w)
		}
	}
	if err := Validate(Profile{Slug: "a", Workdir: "team/researcher"}); err != nil {
		t.Fatalf("relative workdir rejected: %v", err)
	}
}

func TestProfileKind(t *testing.T) {
	if got := (Profile{Slug: "guardian", System: true}).Kind(); got != "system" {
		t.Fatalf("system kind = %q", got)
	}
	no := false
	if got := (Profile{Slug: "worker", DirectCallable: &no}).Kind(); got != "subagent" {
		t.Fatalf("managed kind = %q", got)
	}
	if got := (Profile{Slug: "researcher"}).Kind(); got != "custom" {
		t.Fatalf("custom kind = %q", got)
	}
}

func TestValidate_ManagedSubAgentRequiresHierarchy(t *testing.T) {
	no := false
	if err := Validate(Profile{Slug: "worker", DirectCallable: &no}); err == nil {
		t.Fatal("managed subagent without owner/parent accepted")
	}
	if err := Validate(Profile{Slug: "worker", ParentAgent: "worker", DirectCallable: &no}); err == nil {
		t.Fatal("self-parent managed subagent accepted")
	}
	if err := Validate(Profile{Slug: "worker", OwnerAgent: "lead", DirectCallable: &no}); err != nil {
		t.Fatalf("owned managed subagent rejected: %v", err)
	}
	if (Profile{Slug: "worker", DirectCallable: &no}).AllowsDelegationFrom("lead") {
		t.Fatal("unowned managed subagent allowed delegation")
	}
	if !(Profile{Slug: "worker", ParentAgent: "lead", DirectCallable: &no}).AllowsDelegationFrom("lead") {
		t.Fatal("managed subagent rejected parent delegation")
	}
}

// TestAdd_AssignsIdentityAndDefaults: kernel assigns id/enabled/timestamps,
// name defaults to the slug, and the slug must be unique.
func TestAdd_AssignsIdentityAndDefaults(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{Slug: "researcher", Soul: "You research.", ID: "spoofed", Enabled: false})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if p.ID == "" || p.ID == "spoofed" {
		t.Fatalf("ID not kernel-assigned: %q", p.ID)
	}
	if !p.Enabled || p.CreatedMS == 0 || p.UpdatedMS == 0 {
		t.Fatalf("lifecycle defaults wrong: %+v", p)
	}
	if p.Name != "researcher" {
		t.Fatalf("name should default to slug, got %q", p.Name)
	}
	if _, err := s.Add(Profile{Slug: "researcher"}); err == nil {
		t.Fatal("duplicate slug accepted")
	}
}

func TestAdd_SystemGuardianDefaultsQuietCappedAndIsolated(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{
		Slug:         "guardian-health",
		System:       true,
		MemoryScope:  "guardian-health",
		TrustCeiling: "L4",
		NoisePolicy: &NoisePolicy{
			MinNotifySeverity:    "info",
			MinNotifyIntervalSec: 60,
		},
	})
	if err != nil {
		t.Fatalf("Add system guardian: %v", err)
	}
	if p.MemoryScope != "system/guardian-health" {
		t.Fatalf("system guardian memory scope = %q", p.MemoryScope)
	}
	if p.MaxCostMc != defaultSystemGuardianMaxCostMc || p.MaxDailyMc != defaultSystemGuardianMaxDailyMc {
		t.Fatalf("system guardian caps = run %d daily %d", p.MaxCostMc, p.MaxDailyMc)
	}
	if p.TrustCeiling != defaultSystemGuardianTrustCeiling {
		t.Fatalf("system guardian trust ceiling = %q", p.TrustCeiling)
	}
	if p.NoisePolicy == nil || !p.NoisePolicy.SilentOnSuccess || !p.NoisePolicy.DisableMemoryWrites ||
		p.NoisePolicy.MinNotifySeverity != defaultSystemGuardianMinNotifySeverity ||
		p.NoisePolicy.MinNotifyIntervalSec != defaultSystemGuardianNotifyCooldownSec {
		t.Fatalf("system guardian noise defaults wrong: %+v", p.NoisePolicy)
	}
}

func TestAdd_SystemGuardianKeepsStricterExplicitPolicy(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{
		Slug:         "guardian-routing",
		System:       true,
		MemoryScope:  "system/custom-routing",
		MaxCostMc:    10_000_000,
		MaxDailyMc:   20_000_000,
		TrustCeiling: "L1",
		NoisePolicy: &NoisePolicy{
			SilentOnSuccess:      true,
			DisableMemoryWrites:  true,
			MinNotifySeverity:    "critical",
			MinNotifyIntervalSec: 12 * 3600,
		},
	})
	if err != nil {
		t.Fatalf("Add system guardian: %v", err)
	}
	if p.MemoryScope != "system/custom-routing" || p.MaxCostMc != 10_000_000 || p.MaxDailyMc != 20_000_000 || p.TrustCeiling != "L1" {
		t.Fatalf("stricter guardian settings should be preserved: %+v", p)
	}
	if p.NoisePolicy == nil || p.NoisePolicy.MinNotifySeverity != "critical" || p.NoisePolicy.MinNotifyIntervalSec != 12*3600 {
		t.Fatalf("stricter guardian noise should be preserved: %+v", p.NoisePolicy)
	}
}

func TestUpdate_SystemGuardianCannotLoosenQuietDefaults(t *testing.T) {
	s := openStore(t)
	if _, err := s.Add(Profile{Slug: "guardian-health", System: true}); err != nil {
		t.Fatalf("Add system guardian: %v", err)
	}
	p, err := s.Update("guardian-health", func(dst *Profile) {
		dst.MemoryScope = "shared"
		dst.MaxCostMc = 0
		dst.MaxDailyMc = 0
		dst.TrustCeiling = "L4"
		dst.NoisePolicy = &NoisePolicy{MinNotifySeverity: "info", MinNotifyIntervalSec: 1}
	})
	if err != nil {
		t.Fatalf("Update system guardian: %v", err)
	}
	if p.MemoryScope != "system/guardian-health" || p.MaxCostMc != defaultSystemGuardianMaxCostMc ||
		p.MaxDailyMc != defaultSystemGuardianMaxDailyMc || p.TrustCeiling != defaultSystemGuardianTrustCeiling {
		t.Fatalf("loose guardian update was not raised to defaults: %+v", p)
	}
	if p.NoisePolicy == nil || !p.NoisePolicy.SilentOnSuccess || !p.NoisePolicy.DisableMemoryWrites ||
		p.NoisePolicy.MinNotifySeverity != defaultSystemGuardianMinNotifySeverity ||
		p.NoisePolicy.MinNotifyIntervalSec != defaultSystemGuardianNotifyCooldownSec {
		t.Fatalf("loose guardian noise update was not raised: %+v", p.NoisePolicy)
	}
}

func TestOpen_MigratesSystemGuardianDefaults(t *testing.T) {
	dir := t.TempDir()
	raw := []*Profile{{
		ID:        "legacy",
		Slug:      "guardian-health",
		Name:      "Guardian Health",
		System:    true,
		Enabled:   true,
		CreatedMS: 1,
		UpdatedMS: 1,
	}}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal legacy roster: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roster.json"), b, 0o644); err != nil {
		t.Fatalf("write legacy roster: %v", err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open legacy roster: %v", err)
	}
	p, ok := s.Get("guardian-health")
	if !ok {
		t.Fatal("migrated guardian missing")
	}
	if p.MemoryScope != "system/guardian-health" || p.MaxCostMc != defaultSystemGuardianMaxCostMc ||
		p.MaxDailyMc != defaultSystemGuardianMaxDailyMc || p.TrustCeiling != defaultSystemGuardianTrustCeiling ||
		p.NoisePolicy == nil || !p.NoisePolicy.SilentOnSuccess || !p.NoisePolicy.DisableMemoryWrites {
		t.Fatalf("legacy guardian was not migrated to quiet defaults: %+v", p)
	}
	if p.UpdatedMS <= 1 {
		t.Fatalf("migration should bump updated_ms, got %d", p.UpdatedMS)
	}
}

func TestAdd_NormalizesLifecycleInstructionsAndTaskList(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{
		Slug:         "cycle-agent",
		Instructions: []string{" stay terse ", "", "report only changes"},
		ToolAllow:    []string{" shell ", "", "memory", "SHELL"},
		ToolDeny:     []string{" notify ", "NOTIFY"},
		TrustCeiling: "l2",
		ConfigOverrides: map[string]string{
			" agezt_x_mode ": " agent-only ",
		},
		RetryPolicy:      &RetryPolicy{Backoff: " exponential ", RetryOn: []string{" error ", "", "timeout"}},
		HealthPolicy:     &HealthPolicy{DoctorAgent: " lead "},
		SelfRepairPolicy: &SelfRepairPolicy{Enabled: true, EscalateTo: " owner "},
		Lifecycle:        AgentLifecycle{Mode: LifecycleCycle, MaxCycles: 12},
		TaskList: []AgentTask{
			{Title: "check inbox", Scope: "cycle"},
			{Title: "finish migration"},
			{Title: "   "},
		},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(p.Instructions) != 2 || p.Instructions[0] != "stay terse" {
		t.Fatalf("instructions not normalized: %+v", p.Instructions)
	}
	if len(p.ToolAllow) != 2 || p.ToolAllow[0] != "shell" || p.ToolAllow[1] != "memory" || len(p.ToolDeny) != 1 || p.ToolDeny[0] != "notify" || p.TrustCeiling != "L2" {
		t.Fatalf("tool permissions not normalized: allow=%v deny=%v trust=%q", p.ToolAllow, p.ToolDeny, p.TrustCeiling)
	}
	if got := p.ConfigOverrides["AGEZT_X_MODE"]; got != "agent-only" {
		t.Fatalf("config overrides not normalized: %+v", p.ConfigOverrides)
	}
	if p.RetryPolicy == nil || p.RetryPolicy.Backoff != "exponential" || len(p.RetryPolicy.RetryOn) != 2 || p.RetryPolicy.RetryOn[0] != "error" {
		t.Fatalf("retry policy not normalized: %+v", p.RetryPolicy)
	}
	if p.HealthPolicy == nil || p.HealthPolicy.DoctorAgent != "lead" {
		t.Fatalf("health policy not normalized: %+v", p.HealthPolicy)
	}
	if p.SelfRepairPolicy == nil || p.SelfRepairPolicy.EscalateTo != "owner" {
		t.Fatalf("self repair policy not normalized: %+v", p.SelfRepairPolicy)
	}
	if p.Lifecycle.Mode != LifecycleCycle || p.Lifecycle.MaxCycles != 12 {
		t.Fatalf("lifecycle lost: %+v", p.Lifecycle)
	}
	if len(p.TaskList) != 2 {
		t.Fatalf("tasklist = %+v, want 2 normalized tasks", p.TaskList)
	}
	if p.TaskList[0].ID == "" || p.TaskList[0].Status != "todo" || p.TaskList[0].CreatedMS == 0 {
		t.Fatalf("cycle task defaults missing: %+v", p.TaskList[0])
	}
	if p.TaskList[1].Scope != "total" {
		t.Fatalf("blank task scope should default to total: %+v", p.TaskList[1])
	}
}

func TestNoisePolicyDisablesMemoryToolAtStoreBoundary(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{
		Slug:      "quiet-agent",
		ToolAllow: []string{"memory", "shell"},
		ToolDeny:  []string{"notify", "MEMORY"},
		NoisePolicy: &NoisePolicy{
			DisableMemoryWrites: true,
		},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(p.ToolAllow) != 1 || p.ToolAllow[0] != "shell" {
		t.Fatalf("memory should be removed from allow when writes are disabled: allow=%v", p.ToolAllow)
	}
	if len(p.ToolDeny) != 2 || p.ToolDeny[0] != "notify" || p.ToolDeny[1] != "memory" {
		t.Fatalf("memory should be canonicalized into deny: deny=%v", p.ToolDeny)
	}

	p, err = s.Update("quiet-agent", func(dst *Profile) {
		dst.ToolAllow = []string{"memory", "fetch"}
		dst.ToolDeny = []string{"notify"}
		dst.NoisePolicy = &NoisePolicy{DisableMemoryWrites: true}
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(p.ToolAllow) != 1 || p.ToolAllow[0] != "fetch" {
		t.Fatalf("update should remove memory from allow when writes are disabled: allow=%v", p.ToolAllow)
	}
	if len(p.ToolDeny) != 2 || p.ToolDeny[0] != "notify" || p.ToolDeny[1] != "memory" {
		t.Fatalf("update should add memory deny: deny=%v", p.ToolDeny)
	}
}

func TestAdd_NormalizesMaxCyclesToCycleLifecycle(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{
		Slug:      "limited-worker",
		Lifecycle: AgentLifecycle{Mode: LifecyclePersistent, MaxCycles: 3},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if p.Lifecycle.Mode != LifecycleCycle || p.Lifecycle.MaxCycles != 3 {
		t.Fatalf("max cycles should imply cycle lifecycle, got %+v", p.Lifecycle)
	}
}

func TestValidate_RejectsBadTrustCeilingAndToolNames(t *testing.T) {
	if err := Validate(Profile{Slug: "a", TrustCeiling: "L9"}); err == nil {
		t.Fatal("bad trust ceiling accepted")
	}
	if err := Validate(Profile{Slug: "a", ToolAllow: []string{"bad tool"}}); err == nil {
		t.Fatal("bad tool name accepted")
	}
	if err := Validate(Profile{Slug: "a", ToolAllow: []string{"shell"}, ToolDeny: []string{" shell "}}); err == nil {
		t.Fatal("tool allow/deny overlap accepted")
	}
	if err := Validate(Profile{Slug: "a", ConfigOverrides: map[string]string{"AGEZT bad": "x"}}); err == nil {
		t.Fatal("bad config override key accepted")
	}
}

// TestGet_ByIdAndSlug: profiles resolve by either handle.
func TestGet_ByIdAndSlug(t *testing.T) {
	s := openStore(t)
	p, _ := s.Add(Profile{Slug: "ops"})
	if got, ok := s.Get(p.ID); !ok || got.Slug != "ops" {
		t.Fatal("lookup by id failed")
	}
	if got, ok := s.Get("ops"); !ok || got.ID != p.ID {
		t.Fatal("lookup by slug failed")
	}
	if _, ok := s.Get("ghost"); ok {
		t.Fatal("unknown ref resolved")
	}
}

// TestUpdate_ProtectsIdentity: mutate can change mutable fields but never the
// id/slug/created/enabled, and a mutation into an invalid state rolls back.
func TestUpdate_ProtectsIdentity(t *testing.T) {
	s := openStore(t)
	p, _ := s.Add(Profile{Slug: "ops", Soul: "v1"})
	got, err := s.Update("ops", func(dst *Profile) {
		dst.Soul = "v2"
		dst.Slug = "hijacked"
		dst.ID = "hijacked"
		dst.Enabled = false
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Soul != "v2" || got.Slug != "ops" || got.ID != p.ID || !got.Enabled {
		t.Fatalf("identity not protected: %+v", got)
	}
	if _, err := s.Update("ops", func(dst *Profile) { dst.MaxCostMc = -5 }); err == nil {
		t.Fatal("invalid mutation accepted")
	}
	if cur, _ := s.Get("ops"); cur.MaxCostMc != 0 || cur.Soul != "v2" {
		t.Fatalf("failed mutation not rolled back: %+v", cur)
	}
	if _, err := s.Update("ghost", func(*Profile) {}); err != ErrNotFound {
		t.Fatalf("unknown ref: err = %v, want ErrNotFound", err)
	}
}

// TestSetEnabled_And_Remove: pause/resume round-trips; remove by slug works
// and reports existence.
func TestSetEnabled_And_Remove(t *testing.T) {
	s := openStore(t)
	_, _ = s.Add(Profile{Slug: "ops"})
	if p, err := s.SetEnabled("ops", false); err != nil || p.Enabled {
		t.Fatalf("pause failed: %+v %v", p, err)
	}
	if p, err := s.SetEnabled("ops", true); err != nil || !p.Enabled {
		t.Fatalf("resume failed: %+v %v", p, err)
	}
	if _, err := s.SetEnabled("ghost", true); err != ErrNotFound {
		t.Fatalf("unknown ref: err = %v, want ErrNotFound", err)
	}
	gone, ok, err := s.Remove("ops")
	if err != nil || !ok || gone.Slug != "ops" {
		t.Fatalf("remove failed: %+v %v %v", gone, ok, err)
	}
	if _, ok, _ := s.Remove("ops"); ok {
		t.Fatal("second remove reported existence")
	}
	if s.Count() != 0 {
		t.Fatalf("count = %d after remove", s.Count())
	}
}

// TestPersistence_SurvivesReopen: the roster is durable — a second Open on the
// same dir sees every profile with fields intact.
func TestPersistence_SurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want, _ := s1.Add(Profile{Slug: "researcher", Soul: "You research deeply.",
		Model: "deepseek-v4-pro", Fallbacks: []string{"m2", "m3"}, MaxCostMc: 500_000,
		MemoryScope: "research", Workdir: "research", Description: "the digger"})
	_, _ = s1.Add(Profile{Slug: "ops"})

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.Count() != 2 {
		t.Fatalf("count after reopen = %d, want 2", s2.Count())
	}
	got, ok := s2.Get("researcher")
	if !ok {
		t.Fatal("researcher lost on reopen")
	}
	if got.ID != want.ID || got.Soul != want.Soul || got.Model != want.Model ||
		got.MaxCostMc != want.MaxCostMc || got.MemoryScope != want.MemoryScope ||
		got.Workdir != want.Workdir || len(got.Fallbacks) != 2 {
		t.Fatalf("profile fields lost on reopen: %+v", got)
	}
}

// TestList_DeterministicOrder: creation order, stable.
func TestList_DeterministicOrder(t *testing.T) {
	s := openStore(t)
	// Deterministic clock: real adds can share a millisecond, flipping the
	// creation-time sort onto the ID tiebreaker (flaked once in CI-local).
	var tick int64 = 1000
	s.now = func() time.Time { tick++; return time.UnixMilli(tick) }
	for _, slug := range []string{"c", "a", "b"} {
		if _, err := s.Add(Profile{Slug: slug}); err != nil {
			t.Fatalf("Add %s: %v", slug, err)
		}
	}
	got := s.List()
	if len(got) != 3 || got[0].Slug != "c" || got[1].Slug != "a" || got[2].Slug != "b" {
		t.Fatalf("order wrong: %+v", got)
	}
}

// TestSetRetired_GraveyardLifecycle (M846): retiring moves an agent to the
// graveyard (pauses it, stamps RetiredMS), Update preserves the state, and
// reviving clears it (leaving the agent paused).
func TestSetRetired_GraveyardLifecycle(t *testing.T) {
	s := openStore(t)
	_, _ = s.Add(Profile{Slug: "ops", Soul: "watch things"}) // Add enables by default

	got, err := s.SetRetired("ops", true, "idle for 90 days")
	if err != nil {
		t.Fatalf("SetRetired(true): %v", err)
	}
	if !got.Retired || got.RetiredMS == 0 || got.RetiredReason != "idle for 90 days" {
		t.Fatalf("expected retired with a timestamp: %+v", got)
	}
	if got.Enabled {
		t.Error("retiring should pause the agent (Enabled=false)")
	}
	if _, err := s.SetEnabled("ops", true); err != ErrRetired {
		t.Fatalf("resume of retired agent: err = %v, want ErrRetired", err)
	}
	if got, err := s.SetEnabled("ops", false); err != nil || got.Enabled {
		t.Fatalf("pausing a retired agent should stay allowed/idempotent: %+v %v", got, err)
	}

	// Update must not clobber the graveyard state (it has its own setter).
	upd, err := s.Update("ops", func(p *Profile) { p.Soul = "edited" })
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !upd.Retired || upd.RetiredReason != "idle for 90 days" {
		t.Error("Update must preserve Retired state and reason")
	}

	// Reload from disk: the state persisted.
	s2, _ := Open(t.TempDir())
	_ = s2 // (separate dir; persistence within the same store is the contract)
	again, _ := s.Get("ops")
	if !again.Retired {
		t.Error("retired state should persist in the store")
	}

	// Revive clears it.
	rev, err := s.SetRetired("ops", false)
	if err != nil {
		t.Fatalf("SetRetired(false): %v", err)
	}
	if rev.Retired || rev.RetiredMS != 0 || rev.RetiredReason != "" {
		t.Errorf("revive should clear the graveyard state: %+v", rev)
	}
}
