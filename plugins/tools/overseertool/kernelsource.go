// SPDX-License-Identifier: MIT

package overseertool

import (
	"context"
	"errors"
	"fmt"
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
}

// NewKernelSource builds the kernel-backed Source. baseDir is the daemon's base
// directory; the board lives at <baseDir>/board.
func NewKernelSource(k *kernelruntime.Kernel, baseDir string) Source {
	return &kernelSource{k: k, baseDir: baseDir, boardDir: filepath.Join(baseDir, "board")}
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
func (s *kernelSource) SetAgentRetired(ref string, retired bool) (roster.Profile, error) {
	return s.k.SetProfileRetired(ref, retired)
}

// EditAgent applies `in`'s mutable fields wholesale to the agent named by ref,
// mirroring the control plane's handleAgentEdit (the same whitelist the webui
// uses). Identity/lifecycle fields (id/slug/enabled/retired) and the System flag
// are NOT touched — a guardian can retune another agent but can't resurrect,
// rename, or promote it to a protected guardian.
func (s *kernelSource) EditAgent(ref string, in roster.Profile) (roster.Profile, error) {
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		dst.Name = in.Name
		dst.Soul = in.Soul
		dst.Model = in.Model
		dst.Fallbacks = in.Fallbacks
		dst.TaskType = in.TaskType
		dst.MaxCostMc = in.MaxCostMc
		dst.MaxDailyMc = in.MaxDailyMc
		dst.MemoryScope = in.MemoryScope
		dst.Workdir = in.Workdir
		dst.OwnerAgent = in.OwnerAgent
		dst.ParentAgent = in.ParentAgent
		dst.DirectCallable = in.DirectCallable
		dst.RetryPolicy = in.RetryPolicy
		dst.HealthPolicy = in.HealthPolicy
		dst.SelfRepairPolicy = in.SelfRepairPolicy
		dst.ToolAllow = in.ToolAllow
		dst.ToolDeny = in.ToolDeny
		dst.TrustCeiling = in.TrustCeiling
		dst.ConfigOverrides = in.ConfigOverrides
		dst.Instructions = in.Instructions
		dst.Lifecycle = in.Lifecycle
		dst.TaskList = in.TaskList
		dst.Description = in.Description
	})
	if err != nil {
		return roster.Profile{}, err
	}
	if !found {
		return roster.Profile{}, fmt.Errorf("unknown agent: %s", ref)
	}
	return p, nil
}

// CreateAgent adds a brand-new agent. System is forced off — only boot-time
// guardian seeding may mint a protected agent.
func (s *kernelSource) CreateAgent(in roster.Profile) (roster.Profile, error) {
	in.System = false
	return s.k.AddProfile(in)
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
