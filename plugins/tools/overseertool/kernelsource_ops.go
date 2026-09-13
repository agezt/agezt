// SPDX-License-Identifier: MIT

package overseertool

// Bulk + wake + search + repair operations on the kernelSource:
// BulkSetEnabled + BulkSetRetired + BulkDelete + WakeAgent + SearchAgents
// + RepairAgent + RollbackRouting + ApplyRoutingChain +
// managedSubAgentRepairHint. Carved out of kernelsource.go during the
// Day 174 god-file split so the main file can stay focused on the
// struct + lifecycle + simple wrappers + Edit/Clone/Get/Create/Delete.
// Public API unchanged.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)
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

