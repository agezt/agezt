// Package resume persists a durable "ticket" per in-flight root run so the
// daemon can pick the work back up after a restart — whether the daemon was
// stopped/started, self-updated, or hard-killed. Today a shutdown cancels every
// run (Halt → task.failed(reason=canceled)) and the work is lost; a ticket lets
// the run be re-dispatched with its accumulated conversation so it continues
// from where it left off instead of being abandoned (M1002).
//
// A ticket is written when a root run starts, its conversation snapshot is
// refreshed at each safe iteration boundary, and it is deleted on clean
// termination. If the daemon dies with the ticket still present, the boot-time
// resumer re-dispatches it.
//
// The package is deliberately dependency-light: it imports only kernel/agent
// (for the serialized Message slice). Everything else a run needs at resume
// time — trust ceiling, cost cap, wake context — is flattened to primitives so
// this package never imports kernel/runtime (which imports it), avoiding an
// import cycle. The runtime assembles a Ticket from its context values; the
// resumer (in the daemon, where the context builders live) rebuilds a run
// context from the flattened fields.
package resume

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
func (s *Store) Dir() string { return s.dir }

func safeName(corr string) string {
	// Correlation ids are "run-<ulid>" — already filename-safe — but strip any
	// path syntax defensively so a crafted corr can't escape the directory.
	r := strings.NewReplacer("/", "_", "\\", "_", "..", "_", ":", "_")
	return r.Replace(corr)
}

func (s *Store) path(corr string) string {
	return filepath.Join(s.dir, safeName(corr)+".json")
}

// Put writes (or overwrites) a ticket. It stamps UpdatedAt (and CreatedAt on
// first write) and enforces the size cap: an oversized ticket has its message
// snapshot dropped (SnapshotDropped set) so files stay bounded.
func (s *Store) Put(t *Ticket) error {
	if t == nil || t.Corr == "" {
		return errors.New("resume: ticket requires a correlation id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(t)
}

func (s *Store) putLocked(t *Ticket) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = StatusActive
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("resume: marshal %s: %w", t.Corr, err)
	}
	if len(b) > s.maxBytes && len(t.Messages) > 0 {
		// Too large: drop the conversation snapshot but keep the dispatch
		// metadata so the run can still resume by intent-replay.
		t.Messages = nil
		t.Iter = 0
		t.SnapshotDropped = true
		if b, err = json.MarshalIndent(t, "", "  "); err != nil {
			return fmt.Errorf("resume: marshal %s: %w", t.Corr, err)
		}
	}
	return writeAtomic(s.path(t.Corr), b)
}

// Snapshot refreshes the conversation snapshot of an existing ticket, preserving
// its status and dispatch metadata. No-op (returns nil) if the ticket is gone —
// the run start writes the ticket first, and a race where it was just deleted on
// clean termination must not resurrect it.
func (s *Store) Snapshot(corr string, msgs []agent.Message, iter int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok, err := s.getLocked(corr)
	if err != nil || !ok {
		return err
	}
	t.Messages = msgs
	t.Iter = iter
	t.SnapshotDropped = false
	return s.putLocked(t)
}

// Get loads one ticket. ok is false if no such ticket exists.
func (s *Store) Get(corr string) (*Ticket, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(corr)
}

func (s *Store) getLocked(corr string) (*Ticket, bool, error) {
	b, err := os.ReadFile(s.path(corr))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var t Ticket
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, false, fmt.Errorf("resume: unmarshal %s: %w", corr, err)
	}
	return &t, true, nil
}

// List returns every ticket in the directory (quarantine excluded), sorted by
// CreatedAt so resume is deterministic. A corrupt file is skipped, not fatal.
func (s *Store) List() ([]*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []*Ticket
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var t Ticket
		if json.Unmarshal(b, &t) != nil || t.Corr == "" {
			continue
		}
		out = append(out, &t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// Delete removes a ticket. Absent is not an error (idempotent — RunWith's defer
// and a resumer cleanup can both fire).
func (s *Store) Delete(corr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(corr))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// MarkSuspendedAll flips every active ticket to suspended — the shutdown signal
// that these runs are resume candidates, not completed work. Returns the number
// marked. Tickets already suspended (a prior partial shutdown) are left as-is.
func (s *Store) MarkSuspendedAll() (int, error) {
	tickets, err := s.List()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range tickets {
		if t.Status == StatusSuspended {
			continue
		}
		t.Status = StatusSuspended
		if err := s.putLocked(t); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// IncrementAttempt bumps and durably persists a ticket's attempt counter,
// returning the new count. The resumer MUST call this and observe the fsync
// BEFORE re-dispatching: a resume that hard-crashes the daemon must still have
// recorded the attempt, or the crash-loop guard never trips and the watchdog
// eventually gives up — leaving the daemon down permanently.
func (s *Store) IncrementAttempt(corr string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok, err := s.getLocked(corr)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, os.ErrNotExist
	}
	t.Attempts++
	if err := s.putLocked(t); err != nil {
		return t.Attempts, err
	}
	return t.Attempts, nil
}

// Quarantine moves a poison ticket out of the scan directory so it is never
// re-dispatched but remains on disk for a postmortem. Used when a ticket has
// exceeded its attempt cap or is not resumable.
func (s *Store) Quarantine(corr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.path(corr)
	dst := filepath.Join(s.quarDir, fmt.Sprintf("%s.%d.json", safeName(corr), time.Now().UTC().UnixNano()))
	err := os.Rename(src, dst)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// writeAtomic writes b to path via a temp file that is fsynced before the
// rename, so a crash never yields a torn or unsynced ticket. os.Rename is atomic
// on POSIX and (via MoveFileEx replace-existing) on Windows.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
