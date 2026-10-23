// SPDX-License-Identifier: MIT

package cadence

// The schedule ROW lives in the contract layer (kernel/contract/cadenceapi):
// the introspector and the guardians are PLUGINS, and a plugin may reach the
// contract and the platform layers and nothing else — so a row a plugin has to
// name cannot live in this package. The names below stay as aliases, so every
// existing caller is unchanged; the store, the engine and the ticker stay
// here — they are the implementation.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
)

// The schedule row and the vocabulary around it — the contract's, kept here
// as aliases so the module's public names do not move.
type (
	Entry          = cadenceapi.Entry
	SystemTaskInfo = cadenceapi.SystemTaskInfo
)

const (
	// MinInterval guards against a busy-loop from a misconfigured tiny interval.
	MinInterval = cadenceapi.MinInterval

	// DefaultResolution is how often the ticker wakes to check for due entries.
	DefaultResolution = 10 * time.Second

	// Source values distinguish operator-managed entries from env-seeded ones.
	SourceOperator = cadenceapi.SourceOperator
	SourceEnv      = cadenceapi.SourceEnv

	// Target values distinguish what a schedule fires.
	TargetIntent     = cadenceapi.TargetIntent
	TargetWorkflow   = cadenceapi.TargetWorkflow
	TargetSystemTask = cadenceapi.TargetSystemTask
	TargetTool       = cadenceapi.TargetTool

	// Scheduling modes.
	ModeInterval   = cadenceapi.ModeInterval
	ModeDaily      = cadenceapi.ModeDaily
	ModeOnce       = cadenceapi.ModeOnce
	ModeWindow     = cadenceapi.ModeWindow
	ModeContinuous = cadenceapi.ModeContinuous

	// AllDays is the day-mask meaning "every day".
	AllDays = cadenceapi.AllDays

	// The daemon's own recurring jobs.
	SystemTaskCatalogSync     = cadenceapi.SystemTaskCatalogSync
	SystemTaskArtifactCollect = cadenceapi.SystemTaskArtifactCollect
	SystemTaskMemoryClean     = cadenceapi.SystemTaskMemoryClean
	SystemTaskMemoryTidy      = cadenceapi.SystemTaskMemoryTidy
	SystemTaskLogClean        = cadenceapi.SystemTaskLogClean
	SystemTaskGraveyardScan   = cadenceapi.SystemTaskGraveyardScan
	SystemTaskProfileDistill  = cadenceapi.SystemTaskProfileDistill
)

// The system-task catalog and the day-mask helpers — the contract's, kept
// here as wrappers so the module's public names do not move.
func SystemTasks() []string { return cadenceapi.SystemTasks() }

func SystemTaskInfos() []SystemTaskInfo { return cadenceapi.SystemTaskInfos() }

func IsSystemTask(task string) bool { return cadenceapi.IsSystemTask(task) }

func FormatDays(days int) string { return cadenceapi.FormatDays(days) }

func ParseDays(spec string) (int, error) { return cadenceapi.ParseDays(spec) }
