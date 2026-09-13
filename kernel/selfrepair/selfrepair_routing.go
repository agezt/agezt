// SPDX-License-Identifier: MIT

// Self-repair routing-rollback helpers (M846): the autoRepairRoutingRollback +
// autoRepairRoutingRewrite structs, the journal-driven
// autoRepairLatestForceGeneration + autoRepairLatestRoutingRewrite readers,
// and the routing chain comparison / reason helpers.
// Extracted from selfrepair_fingerprints.go during the Day-202 god-file split.
// Public API unchanged.
package selfrepair

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

type autoRepairRoutingRollback struct {
	TaskType  string
	FromChain []string
	ToChain   []string
	Reason    string
}

func autoRepairRoutingRollbackPlan(k *kernelruntime.Kernel, p roster.Profile, row kernelruntime.RoutingPressureAgent, now time.Time) *autoRepairRoutingRollback {
	if k == nil {
		return nil
	}
	taskType := strings.TrimSpace(row.TaskType)
	if taskType == "" {
		taskType = strings.TrimSpace(p.TaskType)
	}
	if taskType == "" {
		return nil
	}
	probation := autoRepairRoutingRollbackProbation()
	if probation <= 0 {
		return nil
	}
	latest := autoRepairLatestRoutingRewrite(k, p.Slug, taskType, now.Add(-probation).UnixMilli())
	if latest == nil || len(latest.PreviousChain) == 0 || len(latest.NewChain) == 0 {
		return nil
	}
	currentChain := autoRepairCurrentTaskModelChain(k, taskType)
	if !autoRepairSameChain(currentChain, latest.NewChain) {
		return nil
	}
	if autoRepairSameChain(latest.PreviousChain, latest.NewChain) {
		return nil
	}
	return &autoRepairRoutingRollback{
		TaskType:  taskType,
		FromChain: append([]string(nil), latest.NewChain...),
		ToChain:   append([]string(nil), latest.PreviousChain...),
		Reason:    autoRepairRoutingRollbackReason(row, latest.PreviousChain),
	}
}

type autoRepairRoutingRewrite struct {
	TSMS          int64
	TaskType      string
	NewChain      []string
	PreviousChain []string
}

func autoRepairLatestForceGeneration(k *kernelruntime.Kernel, slug, taskType string) int {
	if k == nil {
		return 0
	}
	latestTS := int64(0)
	latestGen := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		phase := plStringMap(pl, "phase")
		if plStringMap(pl, "agent") != slug || (phase != "resolution_applied" && phase != "completed") || plStringMap(pl, "resolution") != "force_chain" || plStringMap(pl, "routing_task_type") != taskType {
			return nil
		}
		gen := plIntAny(pl["routing_force_generation"])
		if gen <= 0 {
			gen = 1
		}
		if e.TSUnixMS >= latestTS {
			latestTS = e.TSUnixMS
			latestGen = gen
		}
		return nil
	})
	return latestGen
}

func autoRepairLatestRoutingRewrite(k *kernelruntime.Kernel, slug, taskType string, cutoffMS int64) *autoRepairRoutingRewrite {
	var latest *autoRepairRoutingRewrite
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.TSUnixMS < cutoffMS {
			return nil
		}
		if e.Subject != "doctor.auto_repair" && e.Subject != "agent.repair" {
			return nil
		}
		if e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil || plStringMap(pl, "agent") != slug {
			return nil
		}
		if plStringMap(pl, "phase") != "completed" || plStringMap(pl, "routing_task_type") != taskType {
			return nil
		}
		newChain := plStringsMap(pl, "routing_task_model_chain")
		prevChain := plStringsMap(pl, "previous_routing_task_model_chain")
		if len(newChain) == 0 || len(prevChain) == 0 {
			return nil
		}
		if latest == nil || e.TSUnixMS >= latest.TSMS {
			latest = &autoRepairRoutingRewrite{
				TSMS:          e.TSUnixMS,
				TaskType:      taskType,
				NewChain:      newChain,
				PreviousChain: prevChain,
			}
		}
		return nil
	})
	return latest
}

func autoRepairCurrentTaskModelChain(k *kernelruntime.Kernel, taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if k == nil || taskType == "" {
		return nil
	}
	type taskModelChainsSource interface {
		TaskModelChainsView() map[string][]string
	}
	gov, ok := k.Provider().(taskModelChainsSource)
	if !ok {
		return nil
	}
	src := gov.TaskModelChainsView()[taskType]
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func autoRepairSameChain(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return len(a) > 0
}

func autoRepairRoutingRollbackReason(row kernelruntime.RoutingPressureAgent, chain []string) string {
	base := autoRepairRoutingReason(row)
	if len(chain) == 0 {
		return base
	}
	return base + " — recurrence after a recent routing rewrite; rolling back to " + strings.Join(chain, " -> ")
}
