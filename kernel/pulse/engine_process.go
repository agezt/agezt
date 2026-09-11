// SPDX-License-Identifier: MIT

// Pulse engine processing: process + flushDigest + publish + briefPayload.
// Code extracted from engine.go during the Day-54 god-file split. Public API unchanged.
package pulse


import (
	"context"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)


// safeFlushDigest flushes the digest with the same panic containment as safePoll —
// a panicking briefing sink in the periodic digest must not crash the daemon (M423).
func (e *Engine) safeFlushDigest() (n int) {
	defer func() { _ = recover() }()
	return e.flushDigest()
}

// process runs one delta through the remaining three stages.
func (e *Engine) process(ctx context.Context, d Delta, tickID string) {
	corr := "pulse-" + ulid.New()

	e.publish(event.KindObserverDelta, "pulse.observer."+d.Source, corr, tickID, map[string]any{
		"source":  d.Source,
		"kind":    d.Kind,
		"summary": d.Summary,
		"before":  d.Before,
		"after":   d.After,
		"hints":   d.Hints,
	})

	sc := e.sal.Score(ctx, d)
	e.publish(event.KindSalienceScored, "pulse.salience", corr, tickID, map[string]any{
		"source":      d.Source,
		"score":       sc.Value,
		"reason":      sc.Reason,
		"disposition": string(sc.Disposition),
	})
	if sc.Disposition == DispDrop {
		return
	}

	// Read the dial and quiet window under the lock — SetDial (M758) and SetQuietHours
	// (M770) can change them live from the control-plane goroutine while this scoring
	// runs on the pulse loop.
	e.mu.Lock()
	dial := e.dial
	quiet := e.quiet
	e.mu.Unlock()
	delivery := Route(dial, sc.Disposition, quiet.Active(e.now()))
	if delivery == DeliverDrop {
		return
	}

	// Initiative (M999): when an observation is ACTIONABLE and the operator has
	// enabled autonomy, emit a distinct event that a standing order can fire on —
	// reusing the governed standing→RunWith→policyHook path. The engine itself never
	// acts (it owns no permissions); it only classifies and emits. The level is read
	// live under the lock, like the dial.
	e.mu.Lock()
	initiative := e.initiative
	e.mu.Unlock()
	actionable := sc.Disposition == DispAlert || sc.Disposition == DispAct || d.Hints["actionable"] == "true"
	branch := "inform"
	switch {
	case actionable && initiative == InitiativeAct:
		branch = "act"
	case actionable && initiative == InitiativeAsk:
		branch = "ask"
	}
	e.publish(event.KindInitiativeTaken, "pulse.initiative", corr, tickID, map[string]any{
		"source": d.Source,
		"branch": branch,
		"reason": sc.Reason,
	})
	// The actionable signal: a separate subject standing orders bind to (mirrors how
	// pulse.observer.<source> already drives guardians). Subject distinguishes
	// act vs ask; the payload carries enough for the fired agent to triage.
	if branch == "act" || branch == "ask" {
		payload := map[string]any{
			"source":    d.Source,
			"kind":      d.Kind,
			"summary":   d.Summary,
			"reason":    sc.Reason,
			"score":     sc.Value,
			"issue_key": d.IssueKey(),
		}
		e.publish(event.KindInitiativeAct, "pulse.initiative."+branch, corr, tickID, payload)
		// Under ask, the act subject was NOT fired — queue the signal for the operator's
		// verdict so it isn't a silent dead-end (M1001). Approval re-emits `payload` onto
		// pulse.initiative.act, the same path act-mode takes.
		if branch == "ask" {
			e.queueAsk(&pendingAsk{
				IssueKey: d.IssueKey(),
				Source:   d.Source,
				Kind:     d.Kind,
				Summary:  d.Summary,
				Reason:   sc.Reason,
				Score:    sc.Value,
				TS:       e.now().UnixMilli(),
				payload:  payload,
			})
		}
	}

	// Mark the issue surfaced so an identical repeat within the TTL is
	// suppressed by novelty.
	e.sal.MarkSeen(d.IssueKey())

	switch delivery {
	case DeliverNow:
		b := composeBrief(d, sc, corr)
		_ = e.sink.Deliver(b)
		e.publish(event.KindBriefingSent, "pulse.briefing", corr, tickID, briefPayload(b))
	case DeliverDigest:
		e.mu.Lock()
		e.digest = append(e.digest, composeBrief(d, sc, corr))
		e.mu.Unlock()
	}
}

// flushDigest coalesces accumulated digest items into one brief and delivers
// it (SPEC-03 §6.2). No-op when empty.
func (e *Engine) flushDigest() int {
	e.mu.Lock()
	items := e.digest
	e.digest = nil
	e.mu.Unlock()
	if len(items) == 0 {
		return 0
	}
	b := composeDigest(items)
	corr := "pulse-" + ulid.New()
	b.CorrelationID = corr
	_ = e.sink.Deliver(b)
	e.publish(event.KindBriefingSent, "pulse.briefing", corr, "", briefPayload(b))
	return len(items)
}

// publish is the journaling helper: durable-before-publish through the bus,
// carrying correlation (per-delta chain) and causation (the originating tick).
func (e *Engine) publish(kind event.Kind, subject, corr, causation string, payload any) (*event.Event, error) {
	if e.bus == nil {
		return nil, nil
	}
	return e.bus.Publish(event.Spec{
		Subject:       subject,
		Kind:          kind,
		Actor:         "pulse",
		CorrelationID: corr,
		CausationID:   causation,
		Payload:       payload,
	})
}

func briefPayload(b Brief) map[string]any {
	return map[string]any{
		"title":       b.Title,
		"body":        b.Body,
		"disposition": string(b.Disposition),
		"issue_key":   b.IssueKey,
		"items":       b.Items,
	}
}

// --- control surface (used by `agt pulse status|pause|resume`) ------------

// Status is the snapshot returned to `agt pulse status`.
type Status struct {
	Running       bool       `json:"running"`
	Paused        bool       `json:"paused"`
	Beats         int64      `json:"beats"`
	Observers     []string   `json:"observers"`
	Removable     []string   `json:"removable"`
	Dial          string     `json:"dial"`
	Initiative    string     `json:"initiative"` // autonomy level (off|ask|act); M999
	Quiet         QuietHours `json:"quiet"`
	CadenceMS     int64      `json:"cadence_ms"`
	LastTickMS    int64      `json:"last_tick_ms"`
	DigestPending int        `json:"digest_pending"`
}

// Status returns a snapshot of the engine for operators.