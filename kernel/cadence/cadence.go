// SPDX-License-Identifier: MIT

// Package cadence is the typed schedule subsystem (autonomy): it wakes agent
// tasks, workflows, daemon maintenance tasks, or approved tools on recurring,
// one-shot, daily, or continuous cadences. It is the timer companion to Pulse's
// event-driven proactivity.
//
// Schedules live in a persistent Store (survives restarts) and are managed by
// the operator over the control plane (`agt schedule add|list|rm|run`).
// Operator-configured AGEZT_SCHEDULE env jobs are synced into the same store at
// startup (source="env"), so both paths share one source of truth. The Engine
// ticks, asks the Store which entries are due, and fires each through a RunFunc;
// a still-running entry is skipped (no overlap). Every firing is journaled
// (schedule.fired), and target execution is attributed back to that schedule.
package cadence

import (
	"sync"
	"time"
)

// MinInterval guards against a busy-loop from a misconfigured tiny interval.
const MinInterval = time.Second

// DefaultResolution is how often the ticker wakes to check for due entries.
const DefaultResolution = 10 * time.Second

// Source values distinguish operator-managed entries from env-seeded ones.
const (
	SourceOperator = "operator"
	SourceEnv      = "env"
)

// Target values distinguish what a schedule fires. The zero value is the
// historical governed agent/intent run, so old stores keep working unchanged.
const (
	TargetIntent     = ""
	TargetWorkflow   = "workflow"
	TargetSystemTask = "system_task"
	TargetTool       = "tool"
)

const (
	SystemTaskCatalogSync     = "catalog_sync"
	SystemTaskArtifactCollect = "artifact_collect"
	SystemTaskMemoryClean     = "memory_clean"
	SystemTaskMemoryTidy      = "memory_tidy"
	SystemTaskLogClean        = "log_clean"
	SystemTaskGraveyardScan   = "graveyard_scan"
	SystemTaskProfileDistill  = "profile_distill"
)

type SystemTaskInfo struct {
	Name                   string `json:"name"`
	Label                  string `json:"label"`
	Description            string `json:"description"`
	Category               string `json:"category,omitempty"`
	Executor               string `json:"executor,omitempty"`
	UsesLLM                bool   `json:"uses_llm"`
	EffectClass            string `json:"effect_class,omitempty"`
	Effect                 string `json:"effect,omitempty"`
	RecommendedIntervalSec int64  `json:"recommended_interval_sec,omitempty"`
}

var systemTaskInfos = []SystemTaskInfo{
	{
		Name:                   SystemTaskCatalogSync,
		Label:                  "Catalog sync",
		Description:            "Download the models.dev catalog, persist it, and reload provider/model metadata.",
		Category:               "catalog",
		Executor:               "daemon",
		EffectClass:            "config_update",
		Effect:                 "Refreshes provider/model metadata from models.dev/api.json without waking an LLM agent.",
		RecommendedIntervalSec: 24 * 3600,
	},
	{
		Name:                   SystemTaskArtifactCollect,
		Label:                  "Artifact collect",
		Description:            "Index offloaded run artifacts so autonomous work remains searchable and inspectable.",
		Category:               "storage",
		Executor:               "daemon",
		EffectClass:            "local_index",
		Effect:                 "Indexes local run artifacts as a typed daemon job; no agent identity is woken.",
		RecommendedIntervalSec: 6 * 3600,
	},
	{
		Name:                   SystemTaskMemoryClean,
		Label:                  "Memory clean",
		Description:            "Run memory maintenance and publish a compact maintenance summary.",
		Category:               "memory",
		Executor:               "daemon",
		EffectClass:            "memory_maintenance",
		Effect:                 "Runs memory maintenance as a typed daemon task rather than an agent wake.",
		RecommendedIntervalSec: 24 * 3600,
	},
	{
		Name:                   SystemTaskMemoryTidy,
		Label:                  "Memory tidy",
		Description:            "Run lightweight memory hygiene without waking an LLM agent.",
		Category:               "memory",
		Executor:               "daemon",
		EffectClass:            "memory_maintenance",
		Effect:                 "Runs lightweight memory hygiene without waking an LLM agent.",
		RecommendedIntervalSec: 12 * 3600,
	},
	{
		Name:                   SystemTaskLogClean,
		Label:                  "Log clean",
		Description:            "Inspect journal/log pressure and publish a compact maintenance summary.",
		Category:               "logs",
		Executor:               "daemon",
		EffectClass:            "log_maintenance",
		Effect:                 "Scans durable journal/log pressure without waking an LLM agent; physical deletion stays disabled for hash-chain safety.",
		RecommendedIntervalSec: 24 * 3600,
	},
	{
		Name:                   SystemTaskGraveyardScan,
		Label:                  "Graveyard scan",
		Description:            "Report retired agents past the configured retention window. Notify-only — it never archives or deletes.",
		Category:               "graveyard",
		Executor:               "daemon",
		EffectClass:            "report_only",
		Effect:                 "Lists graveyard identities older than the retention window and journals an eligibility report; removal stays an explicit operator action (no auto-deletion).",
		RecommendedIntervalSec: 24 * 3600,
	},
	{
		Name:                   SystemTaskProfileDistill,
		Label:                  "Profile distill",
		Description:            "Synthesize the operator's profile (preferences, style, expertise, focus) from accumulated memory so every run knows who it works for.",
		Category:               "memory",
		Executor:               "daemon",
		UsesLLM:                true,
		EffectClass:            "memory_maintenance",
		Effect:                 "Reads accumulated shared memory and writes/refreshes the operator-profile facets via one LLM pass (the 'distill' budgeting class).",
		RecommendedIntervalSec: 24 * 3600,
	},
}

type Job struct {
	Interval time.Duration
	Intent   string
	Model    string
}

// --- Store ---

// Store is the persistent set of schedules, written as a single JSON file
// rewritten atomically on change. It is safe for concurrent use.
type Store struct {
	path    string
	mu      sync.Mutex
	entries []*Entry
}

// OpenStore opens (or creates) the schedule store under dir.