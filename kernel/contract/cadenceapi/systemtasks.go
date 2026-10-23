// SPDX-License-Identifier: MIT

package cadenceapi

// The system-task catalog: the daemon's own recurring jobs, which a schedule
// entry can target instead of an agent run. It is data, and the row that names
// it (the Entry) lives in this package — so the catalog lives here too, and
// `Entry.Validate` can check a system-task target without reaching out of the
// contract.

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
		Effect:                 "Runs lightweight memory hygiene as a typed daemon task rather than an agent wake.",
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
