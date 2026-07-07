// SPDX-License-Identifier: MIT

package overseertool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
)

// kernelSource adapts the live *runtime.Kernel to the overseer's Source. Reads
// and interventions go straight to the kernel's own methods, so each is
// journaled and reversible exactly like its operator-driven equivalent. The
// board is opened fresh per OpenHelp read (mirroring the control plane's board
// view) so the overseer always sees the latest committed help requests without
// sharing the board tool's in-process instance.
type kernelSource struct {
	k        *kernelruntime.Kernel
	baseDir  string
	boardDir string
	// fleetLock, when set, makes the AGENT-reachable EditAgent/CreateAgent paths
	// refuse (V-012). It is an OPT-IN guardrail off by default — the project's
	// default-allow posture is preserved unless an operator sets
	// AGEZT_OVERSEER_FLEET_LOCK. Operator control-plane edits and the auto-repair
	// daemon use other Source methods (RepairAgent/routing), so they are
	// unaffected; only an agent self-administering the fleet via the `overseer`
	// tool is gated.
	fleetLock bool
}

// NewKernelSource builds the kernel-backed Source. baseDir is the daemon's base
// directory; the board lives at <baseDir>/board.
func NewKernelSource(k *kernelruntime.Kernel, baseDir string) Source {
	return &kernelSource{
		k:         k,
		baseDir:   baseDir,
		boardDir:  filepath.Join(baseDir, "board"),
		fleetLock: fleetLockEnabled(),
	}
}

// fleetLockEnabled reports whether agent-initiated fleet administration (editing
// or creating agents via the overseer tool) is locked. Off by default — only an
// explicit truthy AGEZT_OVERSEER_FLEET_LOCK turns it on, so the default-allow
// posture is the default and the restriction is strictly opt-out.
func fleetLockEnabled() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv(brand.EnvPrefix + "OVERSEER_FLEET_LOCK"))) {
	case "1", "on", "true", "yes":
		return true
	default:
		return false
	}
}

func (s *kernelSource) IsHalted() bool         { return s.k.IsHalted() }
func (s *kernelSource) ActiveRunIDs() []string { return s.k.ActiveRunIDs() }
func (s *kernelSource) Agents() []roster.Profile {
	return s.k.Roster().List()
}
func (s *kernelSource) AgentImpact(slug string) []string { return s.k.AgentImpact(slug) }
func (s *kernelSource) CancelRun(corr string) bool       { return s.k.CancelRun(corr) }
func (s *kernelSource) Halt(reason string)               { s.k.HaltWith(reason) }
func (s *kernelSource) ResumeAll(reason string)          { s.k.ResumeWith(reason) }

func (s *kernelSource) SetAgentEnabled(ref string, enabled bool) (roster.Profile, error) {
	return s.k.SetProfileEnabled(ref, enabled)
}
func (s *kernelSource) SetAgentRetired(ref string, retired bool, reason string) (roster.Profile, error) {
	r := strings.TrimSpace(reason)
	if r == "" {
		return s.k.SetProfileRetired(ref, retired)
	}
	return s.k.SetProfileRetired(ref, retired, r)
}

// EditAgent applies `in`'s mutable fields to the agent named by ref, using a
// PATCH semantic: only fields explicitly present in the input JSON payload are
// applied; all other fields remain unchanged. This prevents a partial profile
// (e.g. `{"model":"gpt-5"}` from clearing soul, budget, policy fields, etc.
//
// Identity/lifecycle fields (id/slug/enabled/retired) and the System flag are
// NOT touched — a guardian can retune another agent but can't resurrect,
// rename, or promote it to a protected guardian.
//
// A System-protected guardian (the daemon's own self-healing fleet) cannot be
// edited through this tool at all. The overseer tool is agent-reachable —
// CapOversee is default-allow — so without this guard an arbitrary agent could
// rewrite a guardian's Soul/ToolAllow/ConfigOverrides and behaviorally
// "defang" it even though the System flag itself is preserved (and RemoveProfile
// already refuses to delete it). Operators can still edit guardians through the
// admin control-plane path; only the agent-reachable tool path is restricted.
func (s *kernelSource) EditAgent(ref string, in roster.Profile) (roster.Profile, error) {
	if s.fleetLock {
		return roster.Profile{}, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must make agent edits through the console/CLI")
	}
	cur, ok := s.k.Roster().Get(ref)
	if !ok {
		return roster.Profile{}, fmt.Errorf("unknown agent: %s", ref)
	}
	if cur.System {
		return roster.Profile{}, fmt.Errorf("agent %q is a protected system guardian — it can be retuned only by an operator, not via the overseer tool", cur.Slug)
	}
	// Discover which fields the caller explicitly provided. JSON unmarshal into
	// a flat map tells us the set of top-level keys — nil pointers and zero-value
	// ints/strings come from omission, not from the user intentionally clearing.
	provided := map[string]bool{}
	if raw, err := json.Marshal(in); err == nil {
		var flat map[string]any
		if json.Unmarshal(raw, &flat) == nil {
			for k := range flat {
				provided[k] = true
			}
		}
	}
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		applyProfilePatchField(provided, "name", &dst.Name, in.Name)
		applyProfilePatchField(provided, "soul", &dst.Soul, in.Soul)
		applyProfilePatchField(provided, "model", &dst.Model, in.Model)
		applyProfilePatchField(provided, "task_type", &dst.TaskType, in.TaskType)
		applyProfilePatchField(provided, "memory_scope", &dst.MemoryScope, in.MemoryScope)
		applyProfilePatchField(provided, "workdir", &dst.Workdir, in.Workdir)
		applyProfilePatchField(provided, "owner_agent", &dst.OwnerAgent, in.OwnerAgent)
		applyProfilePatchField(provided, "parent_agent", &dst.ParentAgent, in.ParentAgent)
		applyProfilePatchField(provided, "trust_ceiling", &dst.TrustCeiling, in.TrustCeiling)
		applyProfilePatchField(provided, "description", &dst.Description, in.Description)
		if provided["fallbacks"] {
			dst.Fallbacks = in.Fallbacks
		}
		if provided["max_cost_mc"] {
			dst.MaxCostMc = in.MaxCostMc
		}
		if provided["max_daily_mc"] {
			dst.MaxDailyMc = in.MaxDailyMc
		}
		if provided["direct_callable"] {
			dst.DirectCallable = in.DirectCallable
		}
		if provided["retry_policy"] {
			dst.RetryPolicy = in.RetryPolicy
		}
		if provided["health_policy"] {
			dst.HealthPolicy = in.HealthPolicy
		}
		if provided["self_repair"] {
			dst.SelfRepairPolicy = in.SelfRepairPolicy
		}
		if provided["noise_policy"] {
			dst.NoisePolicy = in.NoisePolicy
		}
		if provided["tool_allow"] {
			dst.ToolAllow = in.ToolAllow
		}
		if provided["tool_deny"] {
			dst.ToolDeny = in.ToolDeny
		}
		if provided["config_overrides"] {
			dst.ConfigOverrides = in.ConfigOverrides
		}
		if provided["instructions"] {
			dst.Instructions = in.Instructions
		}
		if provided["lifecycle"] {
			dst.Lifecycle = in.Lifecycle
		}
		if provided["tasklist"] {
			dst.TaskList = in.TaskList
		}
	})
	if err != nil {
		return roster.Profile{}, err
	}
	if !found {
		return roster.Profile{}, fmt.Errorf("unknown agent: %s", ref)
	}
	return p, nil
}

// applyProfilePatchField sets *dst = val only when key is present in provided.
func applyProfilePatchField[T comparable](provided map[string]bool, key string, dst *T, val T) {
	if provided[key] {
		*dst = val
	}
}

// CreateAgent adds a brand-new agent. System is forced off — only boot-time
// guardian seeding may mint a protected agent.
func (s *kernelSource) CreateAgent(in roster.Profile) (roster.Profile, error) {
	if s.fleetLock {
		return roster.Profile{}, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must create agents through the console/CLI")
	}
	in.System = false
	return s.k.AddProfile(in)
}

// DeleteAgent permanently removes a non-System agent by slug or id. System
// agents are protected — pause or retire them instead. Fleet lock, when set,
// also refuses agent-initiated hard deletes through this tool.
func (s *kernelSource) DeleteAgent(ref string) (bool, error) {
	if s.fleetLock {
		return false, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must delete agents through the console/CLI")
	}
	ok, err := s.k.RemoveProfile(ref)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// GetAgent returns the full profile for a single agent by slug or id.
func (s *kernelSource) GetAgent(ref string) (roster.Profile, bool, error) {
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		return roster.Profile{}, false, nil
	}
	return p, true, nil
}

// CloneAgent creates a new agent from a source profile with caller overrides.
// The source must exist. Override fields (soul/model/fallbacks/task_type/etc.)
// are applied on top. The System flag from the source is never copied. Slug in
// overrides is required. Fleet lock, when set, also refuses cloning through
// this tool.
func (s *kernelSource) CloneAgent(source string, overrides roster.Profile) (roster.Profile, error) {
	if s.fleetLock {
		return roster.Profile{}, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must create agents through the console/CLI")
	}
	src, ok := s.k.Roster().Get(source)
	if !ok {
		return roster.Profile{}, fmt.Errorf("unknown source agent: %s", source)
	}
	// Copy mutable fields from source, then apply overrides on top.
	built := src
	built.System = false // never copy the system flag
	built.ID = ""
	built.CreatedMS = 0
	built.UpdatedMS = 0
	built.Enabled = false // caller must explicitly resume
	built.Retired = false
	built.RetiredMS = 0
	built.RetiredReason = ""

	// Apply overrides
	if v := strings.TrimSpace(overrides.Slug); v != "" {
		built.Slug = v
	}
	if v := strings.TrimSpace(overrides.Name); v != "" {
		built.Name = v
	}
	if overrides.Soul != "" {
		built.Soul = overrides.Soul
	}
	if overrides.Model != "" {
		built.Model = overrides.Model
	}
	if len(overrides.Fallbacks) > 0 {
		built.Fallbacks = append([]string(nil), overrides.Fallbacks...)
	}
	if v := strings.TrimSpace(overrides.TaskType); v != "" {
		built.TaskType = v
	}
	if overrides.MaxCostMc > 0 {
		built.MaxCostMc = overrides.MaxCostMc
	}
	if overrides.MaxDailyMc > 0 {
		built.MaxDailyMc = overrides.MaxDailyMc
	}
	if v := strings.TrimSpace(overrides.MemoryScope); v != "" {
		built.MemoryScope = v
	}
	if v := strings.TrimSpace(overrides.Workdir); v != "" {
		built.Workdir = v
	}
	if v := strings.TrimSpace(overrides.Description); v != "" {
		built.Description = v
	}
	if len(overrides.Instructions) > 0 {
		built.Instructions = append([]string(nil), overrides.Instructions...)
	}
	if v := strings.TrimSpace(overrides.OwnerAgent); v != "" {
		built.OwnerAgent = v
	}
	if v := strings.TrimSpace(overrides.ParentAgent); v != "" {
		built.ParentAgent = v
	}
	if overrides.DirectCallable != nil {
		built.DirectCallable = overrides.DirectCallable
	}
	if len(overrides.ToolAllow) > 0 {
		built.ToolAllow = append([]string(nil), overrides.ToolAllow...)
	}
	if len(overrides.ToolDeny) > 0 {
		built.ToolDeny = append([]string(nil), overrides.ToolDeny...)
	}
	if v := strings.TrimSpace(overrides.TrustCeiling); v != "" {
		built.TrustCeiling = v
	}
	if len(overrides.ConfigOverrides) > 0 {
		if built.ConfigOverrides == nil {
			built.ConfigOverrides = make(map[string]string, len(overrides.ConfigOverrides))
		}
		for k, v := range overrides.ConfigOverrides {
			built.ConfigOverrides[k] = v
		}
	}
	if overrides.RetryPolicy != nil {
		built.RetryPolicy = overrides.RetryPolicy
	}
	if overrides.HealthPolicy != nil {
		built.HealthPolicy = overrides.HealthPolicy
	}
	if overrides.SelfRepairPolicy != nil {
		built.SelfRepairPolicy = overrides.SelfRepairPolicy
	}
	if overrides.NoisePolicy != nil {
		built.NoisePolicy = overrides.NoisePolicy
	}
	if len(overrides.Lifecycle.Mode) > 0 {
		built.Lifecycle = overrides.Lifecycle
	}
	if len(overrides.TaskList) > 0 {
		built.TaskList = append([]roster.AgentTask(nil), overrides.TaskList...)
	}

	return s.k.AddProfile(built)
}

// SearchAgents returns agents matching the filter criteria. Empty filter returns
// all non-retired agents (same as Agents()). Filters are AND-ed.
func (s *kernelSource) BulkSetEnabled(slugs []string, enabled bool) []BulkResult {
	results := make([]BulkResult, 0, len(slugs))
	for _, slug := range slugs {
		_, err := s.k.SetProfileEnabled(slug, enabled)
		r := BulkResult{Slug: slug, Success: err == nil}
		if err != nil {
			r.Error = err.Error()
		}
		results = append(results, r)
	}
	return results
}

func (s *kernelSource) BulkSetRetired(slugs []string, retired bool, reason string) []BulkResult {
	results := make([]BulkResult, 0, len(slugs))
	r := strings.TrimSpace(reason)
	for _, slug := range slugs {
		var err error
		if r == "" {
			_, err = s.k.SetProfileRetired(slug, retired)
		} else {
			_, err = s.k.SetProfileRetired(slug, retired, r)
		}
		res := BulkResult{Slug: slug, Success: err == nil}
		if err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	return results
}

func (s *kernelSource) BulkDelete(slugs []string) []BulkResult {
	results := make([]BulkResult, 0, len(slugs))
	for _, slug := range slugs {
		_, err := s.k.RemoveProfile(slug)
		r := BulkResult{Slug: slug, Success: err == nil}
		if err != nil {
			r.Error = err.Error()
		}
		results = append(results, r)
	}
	return results
}

// WakeAgent explicitly wakes a named agent now, asynchronously, with an
// intent or reason. Validates the agent is enabled and allows direct calls
// before creating the run correlation.
func (s *kernelSource) WakeAgent(ref, intent, reason string) (string, error) {
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		return "", fmt.Errorf("unknown agent: %s", ref)
	}
	if p.Retired {
		return "", fmt.Errorf("agent %s is retired — revive it first", p.Slug)
	}
	if !p.Enabled {
		return "", fmt.Errorf("agent %s is paused", p.Slug)
	}
	if !p.AllowsDirectCall() {
		return "", fmt.Errorf("agent %s is a managed sub-agent and cannot be called directly", p.Slug)
	}
	if strings.TrimSpace(intent) == "" && strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("wake requires an intent or reason")
	}
	if strings.TrimSpace(intent) == "" {
		intent = "wake: " + strings.TrimSpace(reason)
	}
	corr := s.k.NewCorrelation()
	ctx := kernelruntime.WithAgentProfile(context.Background(), p)
	if p.MaxCostMc > 0 {
		ctx = kernelruntime.WithMaxCost(ctx, p.MaxCostMc)
	}
	go func() {
		_, _ = s.k.RunWith(ctx, corr, strings.TrimSpace(intent))
	}()
	return corr, nil
}

func (s *kernelSource) SearchAgents(filter SearchFilter) []roster.Profile {
	all := s.k.Roster().List()
	if len(all) == 0 {
		return nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]roster.Profile, 0, min(len(all), limit))
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	state := strings.ToLower(strings.TrimSpace(filter.State))
	model := strings.ToLower(strings.TrimSpace(filter.Model))
	taskType := strings.ToLower(strings.TrimSpace(filter.TaskType))
	toolAllowed := strings.ToLower(strings.TrimSpace(filter.ToolAllowed))

	for _, p := range all {
		if len(out) >= limit {
			break
		}
		// State filter
		switch state {
		case "enabled":
			if p.Retired || !p.Enabled {
				continue
			}
		case "paused":
			if p.Retired || p.Enabled {
				continue
			}
		case "retired":
			if !p.Retired {
				continue
			}
		}
		// Model filter
		if model != "" && strings.ToLower(strings.TrimSpace(p.Model)) != model {
			continue
		}
		// Task type filter
		if taskType != "" && strings.ToLower(strings.TrimSpace(p.TaskType)) != taskType {
			continue
		}
		// System filter
		if filter.System != nil && p.System != *filter.System {
			continue
		}
		// Owner filter
		if filter.HasOwner != nil {
			has := strings.TrimSpace(p.OwnerAgent) != ""
			if has != *filter.HasOwner {
				continue
			}
		}
		// Parent filter
		if filter.HasParent != nil {
			has := strings.TrimSpace(p.ParentAgent) != ""
			if has != *filter.HasParent {
				continue
			}
		}
		// Tool allowed filter
		if toolAllowed != "" {
			found := false
			for _, t := range p.ToolAllow {
				if strings.EqualFold(strings.TrimSpace(t), toolAllowed) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// Query substring match on slug, name, description
		if query != "" {
			src := strings.ToLower(p.Slug + " " + p.Name + " " + p.Description)
			if !strings.Contains(src, query) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func (s *kernelSource) RepairAgent(ref, reason string) (RepairResult, error) {
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		return RepairResult{}, fmt.Errorf("unknown agent: %s", ref)
	}
	if p.Retired {
		return RepairResult{}, fmt.Errorf("agent %s is retired — revive it first", p.Slug)
	}
	if !p.Enabled {
		return RepairResult{}, fmt.Errorf("agent %s is paused", p.Slug)
	}
	if !p.AllowsDirectCall() {
		return RepairResult{}, errors.New(managedSubAgentRepairHint(p))
	}
	cut := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	report := s.k.ReaperScan(cut, cut)
	taskType := repairTaskType(p, report)
	brief := buildRepairBrief(p, report, reason, s.taskModelChain(taskType))
	corr := s.k.NewCorrelation()
	ctx := kernelruntime.WithAgentProfile(context.Background(), p)
	if p.MaxCostMc > 0 {
		ctx = kernelruntime.WithMaxCost(ctx, p.MaxCostMc)
	}
	var answer string
	var err error
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = s.k.RunWithRetry(ctx, corr, brief, *p.RetryPolicy)
	} else {
		answer, err = s.k.RunWith(ctx, corr, brief)
	}
	if err != nil {
		return RepairResult{}, err
	}
	var applied []string
	var routingTaskType string
	var routingTaskModelChain []string
	var previousRoutingTaskModelChain []string
	if prop := parseRepairProposal(answer); prop != nil {
		if len(prop.TaskModelChain) > 0 && repairProposalTaskType(p, prop) == "" {
			return RepairResult{}, fmt.Errorf("repair proposal included task_model_chain without a task_type or existing agent task type")
		}
		_, _, uerr := s.k.UpdateProfile(p.Slug, func(dst *roster.Profile) {
			applied = applyRepairProposal(dst, prop)
		})
		if uerr != nil {
			return RepairResult{}, uerr
		}
		if len(prop.TaskModelChain) > 0 {
			routingTaskType = repairProposalTaskType(p, prop)
			previousRoutingTaskModelChain = s.taskModelChain(routingTaskType)
			if err := s.setTaskModelChain(routingTaskType, prop.TaskModelChain); err != nil {
				return RepairResult{}, err
			}
			routingTaskModelChain = append([]string(nil), prop.TaskModelChain...)
			applied = append(applied, "task_model_chain")
		}
	}
	return RepairResult{
		Agent:                         p.Slug,
		Correlation:                   corr,
		Applied:                       applied,
		RoutingTaskType:               routingTaskType,
		RoutingTaskModelChain:         routingTaskModelChain,
		PreviousRoutingTaskModelChain: previousRoutingTaskModelChain,
		Answer:                        clip(answer, 1200),
	}, nil
}

func (s *kernelSource) RollbackRouting(ref, taskType string, targetChain []string, reason string) (RepairResult, error) {
	return s.ApplyRoutingChain(ref, taskType, targetChain, reason)
}

func (s *kernelSource) ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (RepairResult, error) {
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		return RepairResult{}, fmt.Errorf("unknown agent: %s", ref)
	}
	if p.Retired {
		return RepairResult{}, fmt.Errorf("agent %s is retired — revive it first", p.Slug)
	}
	if !p.Enabled {
		return RepairResult{}, fmt.Errorf("agent %s is paused", p.Slug)
	}
	if !p.AllowsDirectCall() {
		return RepairResult{}, errors.New(managedSubAgentRepairHint(p))
	}
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return RepairResult{}, fmt.Errorf("routing chain update requires a task type")
	}
	currentChain := s.taskModelChain(taskType)
	if len(currentChain) == 0 {
		return RepairResult{}, fmt.Errorf("routing chain update found no current chain for task type %s", taskType)
	}
	targetChain = sanitizeTaskModelChain(targetChain)
	if len(targetChain) == 0 {
		return RepairResult{}, fmt.Errorf("routing chain update target chain for %s is empty", taskType)
	}
	if strings.EqualFold(strings.Join(currentChain, "\n"), strings.Join(targetChain, "\n")) {
		return RepairResult{}, fmt.Errorf("routing target for %s already matches the current chain", taskType)
	}
	if err := s.setTaskModelChain(taskType, targetChain); err != nil {
		return RepairResult{}, err
	}
	text := "set " + taskType + " chain to " + strings.Join(targetChain, " → ")
	if reason = strings.TrimSpace(reason); reason != "" {
		text += " (" + clip(reason, 220) + ")"
	}
	return RepairResult{
		Agent:                         p.Slug,
		Applied:                       []string{"task_model_chain"},
		RoutingTaskType:               taskType,
		RoutingTaskModelChain:         append([]string(nil), targetChain...),
		PreviousRoutingTaskModelChain: append([]string(nil), currentChain...),
		Answer:                        text,
	}, nil
}

func managedSubAgentRepairHint(p roster.Profile) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route repair through its parent/owner agent"
	if manager != "" {
		hint = "request repair through " + manager
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot be repaired directly; " + hint
}

// OpenHelp opens the board fresh and returns its open help requests. A failure
// to open (no board yet) yields an empty list rather than an error — the
// overseer should still report on everything else.
func (s *kernelSource) OpenHelp(limit int) []board.Message {
	st, err := board.Open(s.boardDir)
	if err != nil {
		return nil
	}
	return st.OpenHelp(limit)
}

type taskModelChainsSource interface {
	TaskModelChainsView() map[string][]string
	SetTaskModelChains(map[string][]string)
}

func (s *kernelSource) taskModelChain(taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return nil
	}
	gov, ok := s.k.Provider().(taskModelChainsSource)
	if !ok {
		return nil
	}
	chains := gov.TaskModelChainsView()
	src := chains[taskType]
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func (s *kernelSource) setTaskModelChain(taskType string, chain []string) error {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return fmt.Errorf("task model chain target task type is empty")
	}
	gov, ok := s.k.Provider().(taskModelChainsSource)
	if !ok {
		return fmt.Errorf("live provider does not support task model chains")
	}
	chains := gov.TaskModelChainsView()
	clean := make([]string, 0, len(chain))
	for _, model := range chain {
		if model = strings.TrimSpace(model); model != "" {
			clean = append(clean, model)
		}
	}
	if len(clean) == 0 {
		return fmt.Errorf("task model chain for %s is empty", taskType)
	}
	chains[taskType] = clean
	gov.SetTaskModelChains(chains)
	if err := persistTaskModelChains(s.baseDir, chains); err != nil {
		return fmt.Errorf("persist task model chains: %w", err)
	}
	return nil
}

func persistTaskModelChains(baseDir string, chains map[string][]string) error {
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		return err
	}
	envName := brand.EnvPrefix + "TASK_MODEL_CHAINS"
	if spec := encodeTaskModelChains(chains); spec != "" {
		store.Set(envName, spec)
	} else {
		store.Remove(envName)
	}
	return store.Save()
}

func encodeTaskModelChains(chains map[string][]string) string {
	keys := make([]string, 0, len(chains))
	for task, models := range chains {
		if strings.TrimSpace(task) == "" || len(models) == 0 {
			continue
		}
		keys = append(keys, task)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, task := range keys {
		models := sanitizeTaskModelChain(chains[task])
		if len(models) == 0 {
			continue
		}
		parts = append(parts, task+"="+strings.Join(models, ","))
	}
	return strings.Join(parts, ";")
}

func sanitizeTaskModelChain(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			out = append(out, model)
		}
	}
	return out
}
