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
