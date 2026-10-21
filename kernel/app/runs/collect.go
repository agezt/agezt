// SPDX-License-Identifier: MIT

package runs

import (
	"encoding/json"
	"strings"

	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/event"
)

// Collect folds a journal into the run rows: one row per correlation, with
// the run's intent, its terminal markers, its spend, its model, its answer
// excerpt and its current phase.
//
// Single forward walk: build the row on task.received, update it on
// task.completed. There is no early stop after N — the limit is applied
// post-sort, since "last N runs" requires knowing all runs first (the
// journal is ordered by seq, not by run start time, and the same run's
// events interleave with others under concurrency).
func Collect(journal func(func(*event.Event) error) error) (map[string]Run, error) {
	runs := map[string]Run{}
	err := journal(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskReceived:
			entry := runs[e.CorrelationID]
			entry.CorrelationID = e.CorrelationID
			entry.StartedUnixMS = e.TSUnixMS
			entry.StartedSeq = e.Seq
			if entry.Phase == "" {
				entry.Phase = "starting"
			}
			// The intent rides on the task.received payload.
			if intent := extractIntent(e.Payload); intent != "" {
				entry.Intent = intent
			}
			// The agent slug rides on it too, when the run was started as a
			// named roster agent.
			if agent := extractAgent(e.Payload); agent != "" {
				entry.Agent = agent
			}
			runs[e.CorrelationID] = entry
		case event.KindTaskCompleted:
			entry := runs[e.CorrelationID]
			entry.CorrelationID = e.CorrelationID
			entry.CompletedUnixMS = e.TSUnixMS
			entry.Completed = true
			if iters := extractIters(e.Payload); iters > 0 {
				entry.Iters = iters
			}
			if prev := extractAnswerPreview(e.Payload); prev != "" {
				entry.AnswerPreview = prev
			}
			runs[e.CorrelationID] = entry
		case event.KindTaskFailed:
			entry := runs[e.CorrelationID]
			entry.CorrelationID = e.CorrelationID
			entry.Failed = true
			entry.FailedUnixMS = e.TSUnixMS
			entry.FailReason = extractReason(e.Payload)
			runs[e.CorrelationID] = entry
		case event.KindTaskAbandoned:
			entry := runs[e.CorrelationID]
			entry.CorrelationID = e.CorrelationID
			entry.Abandoned = true
			runs[e.CorrelationID] = entry
		case event.KindSubAgentSpawned:
			// The spawn event lives under the PARENT correlation; its payload
			// names the CHILD. The parent link goes on the child's row (created
			// if the spawn is seen before the child's task.received), so a
			// sub-agent run knows its lead.
			child, parent := extractSpawnLink(e.Payload)
			if child != "" && parent != "" {
				entry := runs[child]
				entry.CorrelationID = child
				entry.ParentCorrelation = parent
				runs[child] = entry
			}
		case event.KindBudgetConsumed:
			// Attribute spend to its run. The governor stamps each
			// budget.consumed with the spending run's correlation; fold its
			// cost into the EXISTING row only — a budget event for an unknown
			// correlation (an out-of-run governor call) must not conjure a
			// phantom run that would then count as "running" in the stats.
			// task.received always precedes a run's spend, so the row exists by
			// the time we see its budget events.
			if e.CorrelationID == "" {
				return nil
			}
			entry, ok := runs[e.CorrelationID]
			if !ok {
				return nil
			}
			entry.SpentMicrocents += extractCostMicrocents(e.Payload)
			// The model is first-wins: the model the run was initially routed
			// to. A later fallback to another model does not overwrite it.
			if entry.Model == "" {
				if m := extractModel(e.Payload); m != "" {
					entry.Model = m
				}
			}
			runs[e.CorrelationID] = entry
		// Activity phase (last-wins forward, so the final value is the run's
		// CURRENT phase). Guarded on an existing row — a lifecycle event for
		// an unknown correlation must not conjure a phantom run, exactly as
		// budget.consumed above. Only surfaced for still-running runs.
		case event.KindLLMRequest:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				return nil
			}
			entry.Phase = "thinking"
			runs[e.CorrelationID] = entry
		case event.KindToolInvoked:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				return nil
			}
			entry.Phase = "using tool"
			if t := extractTool(e.Payload); t != "" {
				entry.Tool = t
			}
			runs[e.CorrelationID] = entry
		case event.KindToolResult:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				return nil
			}
			entry.Phase = "observing tool"
			if t := extractTool(e.Payload); t != "" {
				entry.Tool = t
			}
			runs[e.CorrelationID] = entry
		case event.KindTaskContinued:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				return nil
			}
			entry.Phase = "continuing"
			runs[e.CorrelationID] = entry
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

// RunRows converts the folded runs into the roster status' run rows. The
// roster status reads the runs through its own row type; the conversion lives
// here, beside the fold that produces them.
func RunRows(rows map[string]Run) []approster.RunRow {
	out := make([]approster.RunRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, approster.RunRow{
			CorrelationID: r.CorrelationID, Intent: r.Intent, Model: r.Model, Agent: r.Agent,
			SpentMicrocents: r.SpentMicrocents, StartedUnixMS: r.StartedUnixMS,
			ParentCorrelation: r.ParentCorrelation,
			Completed:         r.Completed, Failed: r.Failed, Abandoned: r.Abandoned,
		})
	}
	return out
}

// extractIntent pulls "intent" out of a task.received payload.
// Returns "" if missing or malformed — operator-facing rendering
// gracefully shows "(no intent)" rather than crashing.
func extractIntent(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Intent
}

// extractAgent pulls "agent" out of a task.received payload.
// Returns "" if missing or malformed. An empty string indicates a
// chat run started without --agent; a non-empty slug indicates a run
// started as a named roster agent.
func extractAgent(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Agent
}

// extractTool pulls "tool" out of a tool.invoked / tool.result payload.
// Returns "" if missing or malformed — the run then reports its phase
// without a tool name.
func extractTool(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Tool
}

// extractIters pulls "iters" out of a task.completed payload.
// Returns 0 on parse failure for the same reason as extractIntent.
func extractIters(payload json.RawMessage) int {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		// JSON numbers decode as float64 by default; accept either
		// shape so the field's wire type can evolve without breaking.
		Iters float64 `json:"iters"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int(p.Iters)
}

// extractCostMicrocents pulls cost_microcents out of a budget.consumed
// payload. Returns 0 on parse failure or absence, so an unparseable
// spend event contributes nothing rather than crashing the fold. JSON
// numbers decode as float64; int64(...) truncates the fractional part the
// integer microcents never has.
func extractCostMicrocents(payload json.RawMessage) int64 {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		Cost float64 `json:"cost_microcents"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int64(p.Cost)
}

// extractModel pulls the model name out of a budget.consumed payload.
// Returns "" on parse failure or absence, so an unparseable or model-less
// spend event leaves the run's model unset rather than crashing the fold.
func extractModel(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Model
}

// answerPreviewRunes bounds the one-line answer excerpt folded into a run row.
// Short enough for a single inline ↳ line; the full answer is a
// `agt runs show <child>` away.
const answerPreviewRunes = 80

// extractAnswerPreview pulls the "answer" out of a task.completed payload
// and returns a one-line excerpt: newlines/tabs collapsed to single
// spaces, trimmed, truncated to answerPreviewRunes with an ellipsis. Returns ""
// on parse failure or an empty/whitespace answer so the renderer simply omits it.
func extractAnswerPreview(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	one := strings.Join(strings.Fields(p.Answer), " ") // collapse all whitespace runs
	if one == "" {
		return ""
	}
	r := []rune(one)
	if len(r) > answerPreviewRunes {
		return string(r[:answerPreviewRunes]) + "…"
	}
	return one
}

// extractSpawnLink pulls child + parent correlation ids out of a
// subagent.spawned payload. Returns ("","") on parse failure so the
// fold simply skips an unparseable delegation rather than crashing.
func extractSpawnLink(payload json.RawMessage) (child, parent string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.Child, p.Parent
}

// extractReason pulls "reason" out of a task.failed payload — the
// classified failure tag (error|max_iters|canceled|timeout). Returns "" on
// parse failure or absence, so the renderer falls back gracefully.
func extractReason(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Reason
}
