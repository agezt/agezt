// SPDX-License-Identifier: MIT

// Past-runs enumeration: runEntry type, runEntryStatus, and collectRuns (journal walker).
// Code extracted from runs.go during the Day-37 god-file split. Public API unchanged.
package controlplane


import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)


const (
	defaultRunsLimit = 20
	maxRunsLimit     = 1_000
)

type runEntry struct {
	CorrelationID string
	Intent        string
	StartedUnixMS int64
	// StartedSeq is the journal seq of the task.received event;
	// used as a tie-break for the sort when two runs share the
	// same TSUnixMS (the bus's wall-clock resolution is 1ms, so
	// fast back-to-back submissions collide).
	StartedSeq      int64
	CompletedUnixMS int64
	Iters           int
	Completed       bool
	// Failed is set when a task.failed event terminated this run — it
	// started but errored out (provider error, max iters, cancel/timeout)
	// instead of completing (M30). FailedUnixMS / FailReason carry the
	// terminal timestamp and the classified reason for rendering.
	Failed       bool
	FailedUnixMS int64
	FailReason   string
	// Abandoned is set when a task.abandoned event reconciled this run at
	// boot — it was received but never completed in a prior session (M28).
	// Status precedence is Completed > Failed > Abandoned > running, so a
	// run that somehow carries several terminal markers reports the most
	// authoritative one.
	Abandoned bool
	// ParentCorrelation links a sub-agent run to the lead run that
	// delegated it (M41), derived from the parent's subagent.spawned event.
	// Empty for top-level runs. Lets `agt runs` show the delegation tree.
	ParentCorrelation string
	// SpentMicrocents is the sum of this run's budget.consumed cost (M47),
	// folded from the governor's per-call spend events now that they carry
	// the spending run's correlation. Lets `agt runs stats` cost a run — and,
	// via ParentCorrelation, cost a delegation.
	SpentMicrocents int64
	// AnswerPreview is a one-line excerpt of the run's final answer (M52),
	// folded from the M51 task.completed `answer` field. Lets `agt runs show`
	// show what a delegation RESULTED IN inline on its ↳ line, not just its
	// status/cost. Empty for a run that didn't complete with text.
	AnswerPreview string
	// Model is the run's primary (first-routed) model name (M123), folded
	// first-wins from its budget.consumed events. Lets `agt runs` show and
	// filter by model — "which runs used claude-opus?" / "did the fallback
	// route as expected?" — in a multi-provider deployment. Empty for a run
	// that never spent (no model journaled, e.g. the offline mock or a run
	// that errored before its first call).
	Model string
	// Agent is the roster agent slug this run executed as (M73), extracted
	// from the task.received event's payload when the run was started via
	// a roster agent (--agent flag). Empty for ad-hoc chat runs that were
	// not started as a named agent. Lets the Agents gallery distinguish
	// agent runs from plain chat conversations.
	Agent string
	// Phase is the run's CURRENT activity, folded last-wins from its
	// lifecycle events (starting / thinking / using tool / observing tool /
	// continuing). Authoritative because it comes from the full journal, so
	// the live monitors can show what a running run is doing even on a fresh
	// page load — unlike the client-side fold, which only sees the in-memory
	// event buffer. Only meaningful while the run is still running; a terminal
	// run reports its status instead. Tool names the tool in flight, if any.
	Phase string
	Tool  string
}

// runEntryStatus reports a run's terminal status (M61), the single source of
// truth shared by handleRunsList, handleScheduleFires, and the status filter so
// they never disagree. Precedence: completed > failed > abandoned > running.
func runEntryStatus(r *runEntry) string {
	switch {
	case r.Completed:
		return "completed"
	case r.Failed:
		return "failed"
	case r.Abandoned:
		return "abandoned"
	default:
		return "running"
	}
}

// collectRuns walks the given kernel's journal once and folds
// task.received / task.completed / task.failed / task.abandoned
// events into per-correlation runEntry records. Shared by
// handleRunsList (which sorts + limits + renders) and
// handleRunsStats (which aggregates). The fold is identical in
// both so the two surfaces never disagree about a run's status.
// Takes an explicit kernel so the run views can be tenant-scoped
// (M39): the primary kernel for an empty tenant, else the tenant's
// own isolated journal — a tenant sees only its own runs.
func (s *Server) collectRuns(k *runtime.Kernel) (map[string]*runEntry, error) {
	// Single forward walk: build per-correlation entry on
	// task.received, update on task.completed. We don't try to
	// stream early-stop after N — limit is applied post-sort, since
	// "last N runs" requires knowing all runs first (journal order
	// is by seq, not by run start time, and the same run's events
	// are interleaved with others under concurrency).
	runs := map[string]*runEntry{}
	err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskReceived:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.StartedUnixMS = e.TSUnixMS
			entry.StartedSeq = e.Seq
			if entry.Phase == "" {
				entry.Phase = "starting"
			}
			// Pull intent out of the payload — agent.go writes it as
			// {"intent": "..."} on KindTaskReceived (see kernel/agent).
			if intent := extractIntent(e.Payload); intent != "" {
				entry.Intent = intent
			}
			// Extract agent slug if present — agent.go writes it as
			// {"agent": "<slug>"} when run via --agent (M73).
			if agent := extractAgent(e.Payload); agent != "" {
				entry.Agent = agent
			}
		case event.KindTaskCompleted:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				// Completed without received? Only possible if the
				// journal was rotated mid-run; record the half we
				// have so the operator at least sees the chain id.
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.CompletedUnixMS = e.TSUnixMS
			entry.Completed = true
			if iters := extractIters(e.Payload); iters > 0 {
				entry.Iters = iters
			}
			if prev := extractAnswerPreview(e.Payload); prev != "" {
				entry.AnswerPreview = prev
			}
		case event.KindTaskFailed:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.Failed = true
			entry.FailedUnixMS = e.TSUnixMS
			entry.FailReason = extractReason(e.Payload)
		case event.KindTaskAbandoned:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.Abandoned = true
		case event.KindSubAgentSpawned:
			// The spawn event lives under the PARENT correlation; its payload
			// names the CHILD. We attach the parent link to the child's entry
			// (creating it if the spawn is seen before the child's
			// task.received), so a sub-agent run knows its lead (M41).
			child, parent := extractSpawnLink(e.Payload)
			if child != "" && parent != "" {
				entry, ok := runs[child]
				if !ok {
					entry = &runEntry{CorrelationID: child}
					runs[child] = entry
				}
				entry.ParentCorrelation = parent
			}
		case event.KindBudgetConsumed:
			// Attribute spend to its run (M47). The governor stamps each
			// budget.consumed with the spending run's correlation; fold its
			// cost into the EXISTING entry only — a budget event for an
			// unknown correlation (an out-of-run governor call) must not
			// conjure a phantom run that would then count as "running" in
			// stats. task.received always precedes a run's spend, so the
			// entry exists by the time we see its budget events.
			if e.CorrelationID == "" {
				return nil
			}
			if entry, ok := runs[e.CorrelationID]; ok {
				entry.SpentMicrocents += extractCostMicrocents(e.Payload)
				// Record the run's model (M123), first-wins: the model it was
				// initially routed to. A later fallback to another model does
				// not overwrite it — provider-fallback rates live in `agt
				// provider stats` (M99); this field answers "what model was
				// this run", which the first call settles.
				if entry.Model == "" {
					if m := extractModel(e.Payload); m != "" {
						entry.Model = m
					}
				}
			}
		// Activity phase (last-wins forward, so the final value is the run's
		// CURRENT phase). Guarded on an existing entry — a lifecycle event for
		// an unknown correlation must not conjure a phantom run, exactly as
		// budget.consumed above. Only surfaced for still-running runs.
		case event.KindLLMRequest:
			if entry, ok := runs[e.CorrelationID]; ok {
				entry.Phase = "thinking"
			}
		case event.KindToolInvoked:
			if entry, ok := runs[e.CorrelationID]; ok {
				entry.Phase = "using tool"
				if t := extractTool(e.Payload); t != "" {
					entry.Tool = t
				}
			}
		case event.KindToolResult:
			if entry, ok := runs[e.CorrelationID]; ok {
				entry.Phase = "observing tool"
				if t := extractTool(e.Payload); t != "" {
					entry.Tool = t
				}
			}
		case event.KindTaskContinued:
			if entry, ok := runs[e.CorrelationID]; ok {
				entry.Phase = "continuing"
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

// Cursor encode/decode/filter now live in kernel/journal (shared by every
// journal-backed list endpoint). See journal.DecodeCursor / EncodeCursor /
// KeepBeforeCursor / NextCursor and docs/REFACTOR-A1-CONTROLPLANE-PLAN.md.
