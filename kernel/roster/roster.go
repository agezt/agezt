// SPDX-License-Identifier: MIT

// Package roster is the durable agent roster (M783): named, persistent agent
// profiles — an identity ("researcher", "ops-watcher") with its own soul
// (system prompt), model (+ ordered fallbacks), default task type, per-run
// spend ceiling, memory scope, and workspace subdirectory. A profile is the
// durable HOME for everything that until now lived per-run: `agt run --agent
// researcher` runs AS that agent, and future arcs attach per-agent messaging,
// budgets, and tool grants to the same identity.
//
// Storage mirrors kernel/standing: a single JSON file rewritten atomically on
// change, safe for concurrent use; every lifecycle mutation is journaled by
// the kernel (roster.created/updated/removed) so `agt why` can explain how an
// agent came to exist.
package roster

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/ulid"
)

// ErrNotFound is returned for an unknown profile id/slug.
var ErrNotFound = errors.New("roster: profile not found")

// ErrRetired is returned when a caller tries to resume a graveyard agent through
// the pause/resume lifecycle. Graveyard exit is a distinct revive transition.
var ErrRetired = errors.New("roster: profile is retired")

// Profile is one named agent identity. Slug is the address — unique,
// immutable, what operators and (future) other agents refer to it by.
type Profile struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`           // unique handle, e.g. "researcher"
	Name string `json:"name,omitempty"` // human label; defaults to the slug

	// Soul is the agent's system prompt — who it IS. Applied as the run's
	// system override; memory/world/skill injection still layers on top.
	Soul string `json:"soul,omitempty"`

	// Instructions are durable operating rules for this identity. Soul says who
	// the agent is; instructions say how it should work across every wake.
	Instructions []string `json:"instructions,omitempty"`

	// Model is the primary model for this agent's runs (empty = kernel
	// default); Fallbacks is its ordered per-agent fallback chain (reserved
	// for the routing integration arc — stored and validated now so profiles
	// are forward-complete).
	Model     string   `json:"model,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`

	// TaskType is the governor task type this agent's runs default to
	// (e.g. "coding", "research"); empty = unclassified.
	TaskType string `json:"task_type,omitempty"`

	// MaxCostMc is the per-run spend ceiling in USD-microcents (0 = none).
	// Applied as the run's max_cost default; an explicit per-run cap wins.
	MaxCostMc int64 `json:"max_cost_mc,omitempty"`

	// MaxDailyMc is the per-DAY spend ceiling in USD-microcents (0 = none):
	// the Governor meters every completion this agent makes (runs, delegate
	// children, standing firings) against an identity ledger and refuses
	// once today's total reaches the ceiling (M793).
	MaxDailyMc int64 `json:"max_daily_mc,omitempty"`

	// MemoryScope is the agent's private memory scope (M652); empty = the
	// slug, so every named agent gets its own notes by default.
	MemoryScope string `json:"memory_scope,omitempty"`

	// Workdir is a workspace-relative subdirectory this agent works in
	// (reserved for the per-agent sandbox arc). Must be relative and must
	// not escape the workspace.
	Workdir string `json:"workdir,omitempty"`

	// OwnerAgent and ParentAgent model the durable agent hierarchy. OwnerAgent is
	// the supervising/owning agent (or owner's brain) responsible for this
	// profile. ParentAgent is the leader that may delegate to this profile when
	// it is a managed worker/sub-agent. They are slugs, validated syntactically
	// here and resolved by control-plane/runtime call sites when needed.
	OwnerAgent  string `json:"owner_agent,omitempty"`
	ParentAgent string `json:"parent_agent,omitempty"`

	// DirectCallable controls whether operators/schedules/channels may wake this
	// agent directly. nil or true = directly callable (default for old profiles);
	// false = managed sub-agent, callable only through delegation.
	DirectCallable *bool `json:"direct_callable,omitempty"`

	RetryPolicy      *RetryPolicy      `json:"retry_policy,omitempty"`
	HealthPolicy     *HealthPolicy     `json:"health_policy,omitempty"`
	SelfRepairPolicy *SelfRepairPolicy `json:"self_repair,omitempty"`
	NoisePolicy      *NoisePolicy      `json:"noise_policy,omitempty"`
	ToolAllow        []string          `json:"tool_allow,omitempty"`
	ToolDeny         []string          `json:"tool_deny,omitempty"`
	TrustCeiling     string            `json:"trust_ceiling,omitempty"`
	ConfigOverrides  map[string]string `json:"config_overrides,omitempty"`
	Lifecycle        AgentLifecycle    `json:"lifecycle,omitempty"`
	TaskList         []AgentTask       `json:"tasklist,omitempty"`

	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`

	// Retired moves a no-longer-needed agent to the GRAVEYARD (M846): it is kept
	// (recoverable via Revive) but excluded from delegation and marked in the
	// roster — distinct from a temporary pause (Enabled=false) and from a hard
	// Remove. RetiredMS is when it was retired; RetiredReason is the optional
	// operator/doctor/reaper note explaining why the identity was buried.
	Retired       bool   `json:"retired,omitempty"`
	RetiredMS     int64  `json:"retired_ms,omitempty"`
	RetiredReason string `json:"retired_reason,omitempty"`

	// System marks a SHIPPED internal agent (a guardian seeded at boot, M961):
	// part of the daemon's own self-healing fleet, not a user creation. System
	// agents are protected — Remove refuses them and the reaper never flags them
	// — but they can still be paused, retired, and edited like any agent. The
	// flag is kernel-owned: it is set only at seed time and is never accepted
	// from an edit/add payload, so it cannot be spoofed or cleared by a profile
	// write.
	System bool `json:"system,omitempty"`

	CreatedMS int64 `json:"created_ms"`
	UpdatedMS int64 `json:"updated_ms"`
}

type RetryPolicy struct {
	MaxAttempts  int      `json:"max_attempts,omitempty"`
	Backoff      string   `json:"backoff,omitempty"` // fixed | exponential
	BaseDelaySec int      `json:"base_delay_sec,omitempty"`
	MaxDelaySec  int      `json:"max_delay_sec,omitempty"`
	RetryOn      []string `json:"retry_on,omitempty"` // error | timeout | canceled | halted
}

type HealthPolicy struct {
	StaleAfterSec    int    `json:"stale_after_sec,omitempty"`
	FailureWindow    int    `json:"failure_window,omitempty"`
	FailureThreshold int    `json:"failure_threshold,omitempty"`
	DoctorAgent      string `json:"doctor_agent,omitempty"`
}

type SelfRepairPolicy struct {
	Enabled     bool   `json:"enabled,omitempty"`
	MaxAttempts int    `json:"max_attempts,omitempty"`
	EscalateTo  string `json:"escalate_to,omitempty"`
}

type NoisePolicy struct {
	// SilentOnSuccess is an explicit operating contract for UIs/prompts: routine
	// green runs should finish without operator interruption.
	SilentOnSuccess bool `json:"silent_on_success,omitempty"`
	// DisableMemoryWrites removes the memory tool from this agent's effective
	// tool set, making "don't write sweep logs" deterministic instead of prompt-only.
	DisableMemoryWrites bool `json:"disable_memory_writes,omitempty"`
	// MinNotifySeverity gates notify tool calls. Empty means no extra gate;
	// accepted values are info, warning, critical.
	MinNotifySeverity string `json:"min_notify_severity,omitempty"`
	// MinNotifyIntervalSec is a per-agent durable cooldown for notify calls.
	MinNotifyIntervalSec int `json:"min_notify_interval_sec,omitempty"`
}

const (
	LifecyclePersistent       = "persistent"
	LifecycleCycle            = "cycle"
	LifecycleRetireOnComplete = "retire_on_complete"
)

type AgentLifecycle struct {
	// Mode is the agent identity lifecycle. persistent is the default; cycle
	// means the agent expects repeated wakes; retire_on_complete buries it after
	// a successful run.
	Mode string `json:"mode,omitempty"`
	// RetireOnComplete is kept as an explicit flag so older/newer clients can
	// express the behavior without depending only on Mode.
	RetireOnComplete bool `json:"retire_on_complete,omitempty"`
	MaxCycles        int  `json:"max_cycles,omitempty"`
	CompletedCycles  int  `json:"completed_cycles,omitempty"`
	// LastCompletedRun is the correlation of the run whose success last advanced
	// the cycle count. It makes lifecycle completion idempotent per logical run:
	// RunAssured / RunWithRetry can invoke the agent multiple times under ONE
	// correlation (re-running until the work verifies complete), and without this
	// marker each successful inner run would double-count the cycle.
	LastCompletedRun string `json:"last_completed_run,omitempty"`
}

type AgentTask struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope,omitempty"`  // cycle | total
	Status      string `json:"status,omitempty"` // todo | doing | done | blocked | retired
	CreatedMS   int64  `json:"created_ms,omitempty"`
	UpdatedMS   int64  `json:"updated_ms,omitempty"`
}

// Kind returns the profile's durable identity class for UI/API consumers. It is
// derived from behavior-owning fields so the persisted model cannot drift:
// System guardians are system agents; non-direct-callable profiles are managed
// sub-agents; everything else is a user/custom agent.
func (p Profile) Kind() string {
	if p.System {
		return "system"
	}
	if !p.AllowsDirectCall() {
		return "subagent"
	}
	return "custom"
}

// AllowsDirectCall returns the direct-call policy, defaulting old profiles to
// true so existing roster files don't become inaccessible when the field is
// introduced.
func (p Profile) AllowsDirectCall() bool {
	return p.DirectCallable == nil || *p.DirectCallable
}

// AllowsDelegationFrom reports whether caller may run this profile as a
// delegated worker. Direct-callable agents can always be delegated to. Managed
// workers (DirectCallable=false) require a named caller; if parent/owner is set,
// the caller must match one of those slugs.
func (p Profile) AllowsDelegationFrom(caller string) bool {
	if p.AllowsDirectCall() {
		return true
	}
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return false
	}
	parent := strings.TrimSpace(p.ParentAgent)
	owner := strings.TrimSpace(p.OwnerAgent)
	if parent == "" && owner == "" {
		return false
	}
	return caller == parent || caller == owner
}

// slugRe: lowercase, digit-or-letter first, then letters/digits/dot/dash/underscore.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var toolNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var configKeyRe = regexp.MustCompile(`^AGEZT_[A-Z0-9_]+$`)

const (
	maxSoulBytes       = 64 * 1024 // a soul is a prompt, not a novel
	maxFallbacks       = 8
	maxConfigOverrides = 128

	defaultSystemGuardianMaxCostMc         = 50_000_000 // $0.05/run
	defaultSystemGuardianMaxDailyMc        = 50_000_000 // $0.05/day
	defaultSystemGuardianNotifyCooldownSec = 8 * 3600
	defaultSystemGuardianMinNotifySeverity = "warning"
	defaultSystemGuardianTrustCeiling      = "L4"
	defaultSystemGuardianMemoryScopePrefix = "system/"
)

// Validate checks a profile's user-supplied fields (identity/lifecycle fields
// are kernel-assigned and not judged here).
func Validate(p Profile) error {
	if !slugRe.MatchString(p.Slug) {
		return fmt.Errorf("roster: slug must match %s", slugRe)
	}
	if len(p.Soul) > maxSoulBytes {
		return fmt.Errorf("roster: soul exceeds %d bytes", maxSoulBytes)
	}
	if len(p.Instructions) > 64 {
		return errors.New("roster: at most 64 instructions")
	}
	for _, ins := range p.Instructions {
		if len(ins) > 4096 {
			return errors.New("roster: instruction exceeds 4096 bytes")
		}
	}
	if len(p.Fallbacks) > maxFallbacks {
		return fmt.Errorf("roster: at most %d fallback models", maxFallbacks)
	}
	if len(p.ToolAllow) > 256 {
		return errors.New("roster: at most 256 tool_allow entries")
	}
	if len(p.ToolDeny) > 256 {
		return errors.New("roster: at most 256 tool_deny entries")
	}
	if len(p.ConfigOverrides) > maxConfigOverrides {
		return fmt.Errorf("roster: at most %d config_overrides entries", maxConfigOverrides)
	}
	for _, f := range p.Fallbacks {
		if strings.TrimSpace(f) == "" {
			return errors.New("roster: empty fallback model id")
		}
	}
	if p.MaxCostMc < 0 {
		return errors.New("roster: max_cost_mc must be >= 0")
	}
	if p.MaxDailyMc < 0 {
		return errors.New("roster: max_daily_mc must be >= 0")
	}
	for label, names := range map[string][]string{"tool_allow": p.ToolAllow, "tool_deny": p.ToolDeny} {
		for _, name := range names {
			if !toolNameRe.MatchString(strings.TrimSpace(name)) {
				return fmt.Errorf("roster: %s contains invalid tool name %q", label, name)
			}
		}
	}
	allow := map[string]bool{}
	for _, name := range p.ToolAllow {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			allow[name] = true
		}
	}
	for _, name := range p.ToolDeny {
		if trimmed := strings.ToLower(strings.TrimSpace(name)); trimmed != "" && allow[trimmed] {
			return fmt.Errorf("roster: tool %q cannot be both allowed and denied", strings.TrimSpace(name))
		}
	}
	if strings.TrimSpace(p.TrustCeiling) != "" {
		if _, err := edict.ParseTrustLevel(p.TrustCeiling); err != nil {
			return fmt.Errorf("roster: trust_ceiling: %w", err)
		}
	}
	for key, value := range p.ConfigOverrides {
		if !configKeyRe.MatchString(strings.TrimSpace(key)) {
			return fmt.Errorf("roster: config_overrides key %q must match %s", key, configKeyRe)
		}
		if len(value) > 8192 {
			return fmt.Errorf("roster: config_overrides[%s] exceeds 8192 bytes", key)
		}
	}
	if p.Workdir != "" {
		w := filepath.ToSlash(p.Workdir)
		if filepath.IsAbs(p.Workdir) || strings.HasPrefix(w, "/") ||
			w == ".." || strings.HasPrefix(w, "../") || strings.Contains(w, "/../") || strings.HasSuffix(w, "/..") {
			return errors.New("roster: workdir must be a relative path inside the workspace")
		}
	}
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref != "" && !slugRe.MatchString(ref) {
			return fmt.Errorf("roster: %s must match %s", label, slugRe)
		}
		if ref != "" && ref == strings.TrimSpace(p.Slug) {
			return fmt.Errorf("roster: %s cannot point to the same agent", label)
		}
	}
	if !p.AllowsDirectCall() && strings.TrimSpace(p.OwnerAgent) == "" && strings.TrimSpace(p.ParentAgent) == "" {
		return errors.New("roster: managed sub-agents require owner_agent or parent_agent")
	}
	if p.RetryPolicy != nil {
		if err := validateRetryPolicy(*p.RetryPolicy); err != nil {
			return err
		}
	}
	if p.HealthPolicy != nil {
		if err := validateHealthPolicy(*p.HealthPolicy); err != nil {
			return err
		}
	}
	if p.SelfRepairPolicy != nil {
		if err := validateSelfRepairPolicy(*p.SelfRepairPolicy); err != nil {
			return err
		}
	}
	if p.NoisePolicy != nil {
		if err := validateNoisePolicy(*p.NoisePolicy); err != nil {
			return err
		}
	}
	if err := validateLifecycle(p.Lifecycle); err != nil {
		return err
	}
	if err := validateTaskList(p.TaskList); err != nil {
		return err
	}
	return nil
}

func normalizeProfile(p *Profile, nowMS int64) {
	p.Slug = strings.TrimSpace(p.Slug)
	p.Name = strings.TrimSpace(p.Name)
	p.Soul = strings.TrimSpace(p.Soul)
	p.TaskType = strings.TrimSpace(p.TaskType)
	p.Model = strings.TrimSpace(p.Model)
	p.MemoryScope = strings.TrimSpace(p.MemoryScope)
	p.Workdir = strings.TrimSpace(p.Workdir)
	p.OwnerAgent = strings.TrimSpace(p.OwnerAgent)
	p.ParentAgent = strings.TrimSpace(p.ParentAgent)
	p.Description = strings.TrimSpace(p.Description)
	p.Instructions = compactStrings(p.Instructions)
	p.ToolAllow = compactUniqueStrings(p.ToolAllow)
	p.ToolDeny = compactUniqueStrings(p.ToolDeny)
	p.TrustCeiling = strings.TrimSpace(strings.ToUpper(p.TrustCeiling))
	normalizeProfilePolicies(p)
	if len(p.ConfigOverrides) > 0 {
		out := make(map[string]string, len(p.ConfigOverrides))
		for key, value := range p.ConfigOverrides {
			key = strings.TrimSpace(strings.ToUpper(key))
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(value)
		}
		if len(out) == 0 {
			p.ConfigOverrides = nil
		} else {
			p.ConfigOverrides = out
		}
	}
	mode := strings.TrimSpace(p.Lifecycle.Mode)
	if mode == "" && p.Lifecycle.RetireOnComplete {
		mode = LifecycleRetireOnComplete
	}
	if (mode == "" || mode == LifecyclePersistent) && p.Lifecycle.MaxCycles > 0 {
		mode = LifecycleCycle
	}
	p.Lifecycle.Mode = mode
	out := make([]AgentTask, 0, len(p.TaskList))
	for _, t := range p.TaskList {
		t.ID = strings.TrimSpace(t.ID)
		t.Title = strings.TrimSpace(t.Title)
		t.Description = strings.TrimSpace(t.Description)
		t.Scope = strings.TrimSpace(t.Scope)
		if t.Scope == "" {
			t.Scope = "total"
		}
		t.Status = strings.TrimSpace(t.Status)
		if t.Status == "" {
			t.Status = "todo"
		}
		if t.Title == "" {
			continue
		}
		if t.ID == "" {
			t.ID = ulid.New()
		}
		if t.CreatedMS == 0 {
			t.CreatedMS = nowMS
		}
		t.UpdatedMS = nowMS
		out = append(out, t)
	}
	p.TaskList = out
	systemDefaultsChanged := applySystemGuardianDefaults(p)
	noiseToolsChanged := enforceNoiseToolDeny(p)
	if systemDefaultsChanged || noiseToolsChanged {
		p.UpdatedMS = nowMS
	}
}

func enforceNoiseToolDeny(p *Profile) bool {
	if p == nil || p.NoisePolicy == nil || !p.NoisePolicy.DisableMemoryWrites {
		return false
	}
	changed := false
	allow := make([]string, 0, len(p.ToolAllow))
	for _, tool := range p.ToolAllow {
		if strings.EqualFold(strings.TrimSpace(tool), "memory") {
			changed = true
			continue
		}
		allow = append(allow, tool)
	}
	deny := make([]string, 0, len(p.ToolDeny)+1)
	hasMemory := false
	for _, tool := range p.ToolDeny {
		if strings.EqualFold(strings.TrimSpace(tool), "memory") {
			if !hasMemory {
				deny = append(deny, "memory")
				hasMemory = true
			}
			if tool != "memory" {
				changed = true
			}
			continue
		}
		deny = append(deny, tool)
	}
	if !hasMemory {
		deny = append(deny, "memory")
		changed = true
	}
	if changed {
		p.ToolAllow = allow
		p.ToolDeny = compactUniqueStrings(deny)
	}
	return changed
}

func applySystemGuardianDefaults(p *Profile) bool {
	if p == nil || !p.System {
		return false
	}
	changed := false
	wantScope := defaultSystemGuardianMemoryScopePrefix + strings.TrimSpace(p.Slug)
	if wantScope != defaultSystemGuardianMemoryScopePrefix {
		scope := strings.TrimSpace(p.MemoryScope)
		if scope == "" || !strings.HasPrefix(scope, defaultSystemGuardianMemoryScopePrefix) {
			p.MemoryScope = wantScope
			changed = true
		}
	}
	if p.MaxCostMc <= 0 {
		p.MaxCostMc = defaultSystemGuardianMaxCostMc
		changed = true
	}
	if p.MaxDailyMc <= 0 {
		p.MaxDailyMc = defaultSystemGuardianMaxDailyMc
		changed = true
	}
	if strings.TrimSpace(p.TrustCeiling) == "" {
		p.TrustCeiling = defaultSystemGuardianTrustCeiling
		changed = true
	}
	if p.NoisePolicy == nil {
		p.NoisePolicy = &NoisePolicy{}
		changed = true
	}
	if !p.NoisePolicy.SilentOnSuccess {
		p.NoisePolicy.SilentOnSuccess = true
		changed = true
	}
	if !p.NoisePolicy.DisableMemoryWrites {
		p.NoisePolicy.DisableMemoryWrites = true
		changed = true
	}
	if noiseSeverityRank(p.NoisePolicy.MinNotifySeverity) < noiseSeverityRank(defaultSystemGuardianMinNotifySeverity) {
		p.NoisePolicy.MinNotifySeverity = defaultSystemGuardianMinNotifySeverity
		changed = true
	}
	if p.NoisePolicy.MinNotifyIntervalSec < defaultSystemGuardianNotifyCooldownSec {
		p.NoisePolicy.MinNotifyIntervalSec = defaultSystemGuardianNotifyCooldownSec
		changed = true
	}
	return changed
}

func noiseSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func normalizeProfilePolicies(p *Profile) {
	if p.RetryPolicy != nil {
		p.RetryPolicy.Backoff = strings.TrimSpace(p.RetryPolicy.Backoff)
		p.RetryPolicy.RetryOn = compactStrings(p.RetryPolicy.RetryOn)
	}
	if p.HealthPolicy != nil {
		p.HealthPolicy.DoctorAgent = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
	}
	if p.SelfRepairPolicy != nil {
		p.SelfRepairPolicy.EscalateTo = strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	if p.NoisePolicy != nil {
		p.NoisePolicy.MinNotifySeverity = strings.ToLower(strings.TrimSpace(p.NoisePolicy.MinNotifySeverity))
	}
}

func compactStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func compactUniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func validateLifecycle(l AgentLifecycle) error {
	mode := strings.TrimSpace(l.Mode)
	switch mode {
	case "", LifecyclePersistent, LifecycleCycle, LifecycleRetireOnComplete:
	default:
		return errors.New("roster: lifecycle.mode must be persistent, cycle, or retire_on_complete")
	}
	if l.MaxCycles < 0 {
		return errors.New("roster: lifecycle.max_cycles must be >= 0")
	}
	if l.CompletedCycles < 0 {
		return errors.New("roster: lifecycle.completed_cycles must be >= 0")
	}
	return nil
}

func validateTaskList(tasks []AgentTask) error {
	if len(tasks) > 200 {
		return errors.New("roster: at most 200 agent tasks")
	}
	for _, t := range tasks {
		if strings.TrimSpace(t.Title) == "" {
			return errors.New("roster: tasklist title required")
		}
		if len(t.Title) > 256 {
			return errors.New("roster: tasklist title exceeds 256 bytes")
		}
		if len(t.Description) > 4096 {
			return errors.New("roster: tasklist description exceeds 4096 bytes")
		}
		switch strings.TrimSpace(t.Scope) {
		case "", "cycle", "total":
		default:
			return errors.New("roster: tasklist scope must be cycle or total")
		}
		switch strings.TrimSpace(t.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			return errors.New("roster: tasklist status must be todo, doing, done, blocked, or retired")
		}
		if strings.ContainsAny(t.ID, " \t\r\n") || len(t.ID) > 128 {
			return errors.New("roster: tasklist id must be a compact id")
		}
	}
	return nil
}

func validateRetryPolicy(p RetryPolicy) error {
	if p.MaxAttempts < 0 || p.MaxAttempts > 10 {
		return errors.New("roster: retry_policy.max_attempts must be 0..10")
	}
	if p.BaseDelaySec < 0 || p.BaseDelaySec > 3600 {
		return errors.New("roster: retry_policy.base_delay_sec must be 0..3600")
	}
	if p.MaxDelaySec < 0 || p.MaxDelaySec > 86400 {
		return errors.New("roster: retry_policy.max_delay_sec must be 0..86400")
	}
	if p.MaxDelaySec > 0 && p.BaseDelaySec > p.MaxDelaySec {
		return errors.New("roster: retry_policy.base_delay_sec must be <= max_delay_sec")
	}
	backoff := strings.TrimSpace(p.Backoff)
	if backoff != "" && backoff != "fixed" && backoff != "exponential" {
		return errors.New("roster: retry_policy.backoff must be fixed or exponential")
	}
	for _, r := range p.RetryOn {
		switch strings.TrimSpace(r) {
		case "error", "timeout", "canceled", "halted":
		default:
			return errors.New("roster: retry_policy.retry_on values must be error, timeout, canceled, or halted")
		}
	}
	return nil
}

func validateHealthPolicy(p HealthPolicy) error {
	if p.StaleAfterSec < 0 || p.FailureWindow < 0 || p.FailureThreshold < 0 {
		return errors.New("roster: health_policy numeric fields must be >= 0")
	}
	if p.DoctorAgent != "" && !slugRe.MatchString(strings.TrimSpace(p.DoctorAgent)) {
		return fmt.Errorf("roster: health_policy.doctor_agent must match %s", slugRe)
	}
	return nil
}

func validateSelfRepairPolicy(p SelfRepairPolicy) error {
	if p.MaxAttempts < 0 || p.MaxAttempts > 10 {
		return errors.New("roster: self_repair.max_attempts must be 0..10")
	}
	if p.EscalateTo != "" && !slugRe.MatchString(strings.TrimSpace(p.EscalateTo)) {
		return fmt.Errorf("roster: self_repair.escalate_to must match %s", slugRe)
	}
	return nil
}

func validateNoisePolicy(p NoisePolicy) error {
	if p.MinNotifyIntervalSec < 0 || p.MinNotifyIntervalSec > 30*24*3600 {
		return errors.New("roster: noise_policy.min_notify_interval_sec must be 0..2592000")
	}
	switch strings.ToLower(strings.TrimSpace(p.MinNotifySeverity)) {
	case "", "info", "warning", "critical":
	default:
		return errors.New("roster: noise_policy.min_notify_severity must be info, warning, or critical")
	}
	return nil
}

// Store is the persistent roster, a single JSON file rewritten atomically on
// change. Safe for concurrent use. Mirrors kernel/standing.Store.
type Store struct {
	path     string
	mu       sync.Mutex
	now      func() time.Time
	profiles []*Profile
}

// Open opens (or creates) the roster store under dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("roster: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "roster.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("roster: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.profiles); err != nil {
			return nil, fmt.Errorf("roster: parse %s: %w", s.path, err)
		}
		changed := false
		for _, p := range s.profiles {
			if applySystemGuardianDefaults(p) {
				p.UpdatedMS = s.now().UnixMilli()
				changed = true
			}
		}
		if changed {
			if err := s.save(); err != nil {
				return nil, fmt.Errorf("roster: migrate %s: %w", s.path, err)
			}
		}
	}
	return s, nil
}

// Add validates and persists a new enabled profile, assigning an id +
// timestamps. Caller-supplied ID/Enabled/timestamps are ignored
// (kernel-assigned). The slug must be unique across the roster.
func (s *Store) Add(p Profile) (Profile, error) {
	now := s.now().UnixMilli()
	normalizeProfile(&p, now)
	if err := Validate(p); err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.profiles {
		if ex.Slug == p.Slug {
			return Profile{}, fmt.Errorf("roster: slug %q already exists", p.Slug)
		}
	}
	p.ID = ulid.New()
	if strings.TrimSpace(p.Name) == "" {
		p.Name = p.Slug
	}
	p.Enabled = true
	p.CreatedMS = now
	p.UpdatedMS = now
	cp := p
	s.profiles = append(s.profiles, &cp)
	if err := s.save(); err != nil {
		s.profiles = s.profiles[:len(s.profiles)-1]
		return Profile{}, err
	}
	return cp, nil
}

// SetEnabled pauses (false) or resumes (true) a profile by id or slug.
func (s *Store) SetEnabled(ref string, enabled bool) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.find(ref)
	if p == nil {
		return Profile{}, ErrNotFound
	}
	if enabled && p.Retired {
		return Profile{}, ErrRetired
	}
	// Roll back the in-memory mutation if the durable write fails, so the
	// running view never diverges from disk on a transient save error.
	prevEnabled, prevUpdated := p.Enabled, p.UpdatedMS
	p.Enabled = enabled
	p.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		p.Enabled, p.UpdatedMS = prevEnabled, prevUpdated
		return Profile{}, err
	}
	return *p, nil
}

// SetRetired moves a profile to the graveyard (true) or revives it (false) by id
// or slug (M846). Retiring also pauses the agent (Enabled=false) so it stops
// firing; reviving leaves it paused for the operator to explicitly resume.
func (s *Store) SetRetired(ref string, retired bool, reason ...string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.find(ref)
	if p == nil {
		return Profile{}, ErrNotFound
	}
	snapshot := *p
	p.Retired = retired
	if retired {
		p.RetiredMS = s.now().UnixMilli()
		if len(reason) > 0 {
			p.RetiredReason = strings.TrimSpace(reason[0])
		}
		p.Enabled = false // a graveyard agent does not run
	} else {
		p.RetiredMS = 0
		p.RetiredReason = ""
	}
	p.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		*p = snapshot
		return Profile{}, err
	}
	return *p, nil
}

// Update applies edits to a profile's mutable fields via mutate, re-validates,
// and persists. Identity and lifecycle fields — ID, Slug, CreatedMS, Enabled
// (which has its own setter) — are preserved regardless of what mutate does;
// UpdatedMS is bumped. Rolled back in memory on validation/save failure.
func (s *Store) Update(ref string, mutate func(*Profile)) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.find(ref)
	if p == nil {
		return Profile{}, ErrNotFound
	}
	snapshot := *p
	mutate(p)
	// Protect identity + lifecycle fields from the mutator. The slug is the
	// agent's ADDRESS — renaming it would orphan every reference to it.
	p.ID, p.Slug, p.CreatedMS, p.Enabled = snapshot.ID, snapshot.Slug, snapshot.CreatedMS, snapshot.Enabled
	p.Retired, p.RetiredMS, p.RetiredReason = snapshot.Retired, snapshot.RetiredMS, snapshot.RetiredReason // graveyard state has its own setter (M846)
	now := s.now().UnixMilli()
	normalizeProfile(p, now)
	p.UpdatedMS = now
	if err := Validate(*p); err != nil {
		*p = snapshot
		return Profile{}, err
	}
	if err := s.save(); err != nil {
		*p = snapshot
		return Profile{}, err
	}
	return *p, nil
}

// Remove deletes a profile by id or slug. Returns whether it existed.
func (s *Store) Remove(ref string) (Profile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.profiles {
		if p.ID == ref || p.Slug == ref {
			removed := s.profiles
			gone := *p
			s.profiles = append(append([]*Profile{}, s.profiles[:i]...), s.profiles[i+1:]...)
			if err := s.save(); err != nil {
				s.profiles = removed // restore: disk write failed, keep the profile
				return Profile{}, false, err
			}
			return gone, true, nil
		}
	}
	return Profile{}, false, nil
}

// Get returns one profile by id or slug.
func (s *Store) Get(ref string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.find(ref); p != nil {
		return *p, true
	}
	return Profile{}, false
}

// find returns the live pointer for an id or slug. Caller holds s.mu.
func (s *Store) find(ref string) *Profile {
	for _, p := range s.profiles {
		if p.ID == ref || p.Slug == ref {
			return p
		}
	}
	return nil
}

// List returns all profiles, sorted by creation time then id (deterministic).
func (s *Store) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, 0, len(s.profiles))
	for _, p := range s.profiles {
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of profiles.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.profiles)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
