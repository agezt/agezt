// SPDX-License-Identifier: MIT

// Package resume persists a durable "ticket" per in-flight root run so the
// daemon can pick the work back up after a restart (M1002).

// This file holds the declarations: Kind/Status consts +
// DefaultSnapshotMaxBytes const + Ticket + Store types + Open
// constructor. Split from resume.go during Day 211 god-file
// refactor (#48). Public API unchanged.
package resume

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)


// Kind classifies how the run was dispatched, so the resumer re-enters through
// the matching governed entry point.
const (
	KindRun     = "run"     // plain RunWith — resume by seeding the saved messages
	KindAssured = "assured" // RunAssured — resume by re-dispatching the wrapper (re-verifies)
	KindRetry   = "retry"   // RunWithRetry — resume by re-dispatching the wrapper
)

// Status is a ticket's lifecycle state.
const (
	StatusActive    = "active"    // run is (or was) live
	StatusSuspended = "suspended" // daemon is going down; this run is a resume candidate
)

// DefaultSnapshotMaxBytes caps a serialized ticket. A long run's full message
// history (tool outputs are retained untruncated in the loop) can reach
// megabytes; past this cap the snapshot is dropped and the run resumes by
// intent-replay rather than bloating the disk.
const DefaultSnapshotMaxBytes = 2 << 20 // 2 MiB

// Ticket is the durable record of one root run.
type Ticket struct {
	Corr         string `json:"corr"`
	Intent       string `json:"intent"`
	AgentSlug    string `json:"agent_slug,omitempty"`
	Kind         string `json:"kind"`
	AssureBudget int    `json:"assure_budget,omitempty"`

	// Resolved run context — the effective values a resumed run must run under,
	// captured so resume neither loses a tightened trust ceiling nor guesses a
	// cap. TrustCeiling is a *int (the underlying edict.TrustLevel value): nil
	// means no ceiling was ever set (LevelAllow), which is legitimate; a set
	// value MUST be re-applied on resume so authority is never silently regained.
	TrustCeiling *int  `json:"trust_ceiling,omitempty"`
	MaxCostMc    int64 `json:"max_cost_mc,omitempty"`
	RunTimeoutMs int64 `json:"run_timeout_ms,omitempty"`

	// Wake context, flattened (see package doc for why it isn't the runtime type).
	WakeSource         string `json:"wake_source,omitempty"`
	WakeReason         string `json:"wake_reason,omitempty"`
	WakeScheduleID     string `json:"wake_schedule_id,omitempty"`
	WakeStandingID     string `json:"wake_standing_id,omitempty"`
	WakeStandingName   string `json:"wake_standing_name,omitempty"`
	WakeTriggerSubject string `json:"wake_trigger_subject,omitempty"`

	// Continuity snapshot. Messages is the loop's in-flight conversation as of
	// Iter, captured at a safe boundary (a complete assistant→tool set). Dropped
	// (with SnapshotDropped set) if the ticket would exceed the size cap.
	Messages        []agent.Message `json:"messages,omitempty"`
	Iter            int             `json:"iter,omitempty"`
	SnapshotDropped bool            `json:"snapshot_dropped,omitempty"`

	// Resumable is false when the run used a per-run override this ticket can't
	// faithfully reconstruct (ad-hoc system prompt, tool allowlist, or model).
	// Such a ticket is cleaned up on boot, not re-dispatched — resuming under the
	// wrong constraints is worse than not resuming.
	Resumable bool `json:"resumable"`

	Status    string    `json:"status"`
	Attempts  int       `json:"attempts,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists tickets as one JSON file per run under dir, with a quarantine
// subdirectory for poison tickets. All writes are atomic (tmp + fsync + rename)
// so a crash never leaves a torn ticket and the crash-loop attempt counter is
// durable. Safe for concurrent use.
type Store struct {
	dir      string
	quarDir  string
	maxBytes int
	mu       sync.Mutex
}

// Open prepares the ticket directory (and its quarantine subdir). maxBytes <= 0
// uses DefaultSnapshotMaxBytes.
func Open(dir string, maxBytes int) (*Store, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultSnapshotMaxBytes
	}
	quar := filepath.Join(dir, "quarantine")
	if err := os.MkdirAll(quar, 0o700); err != nil {
		return nil, fmt.Errorf("resume: open %s: %w", dir, err)
	}
	return &Store{dir: dir, quarDir: quar, maxBytes: maxBytes}, nil
}

// Dir reports the ticket directory (for logging/tests).
