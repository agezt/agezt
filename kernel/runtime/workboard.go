// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/workboard"
)

// Workboard returns the durable typed multi-agent work queue.
func (k *Kernel) Workboard() *workboard.Store { return k.workboard }

func (k *Kernel) CreateWorkboardTask(corr string, spec workboard.CreateSpec) (workboard.Task, bool, error) {
	t, created, err := k.workboard.Create(spec, time.Now())
	if err != nil {
		return workboard.Task{}, false, err
	}
	if created {
		k.publishWorkboard(corr, event.KindWorkboardTaskCreated, t, "created", nil)
	}
	return t, created, nil
}

func (k *Kernel) ClaimWorkboardTask(corr, id, agent, runID string) (workboard.Task, error) {
	t, err := k.workboard.Claim(id, agent, runID, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskClaimed, t, "claimed", map[string]any{"agent": agent, "run_id": runID})
	return t, nil
}

func (k *Kernel) HeartbeatWorkboardTask(corr, id, agent, runID string) (workboard.Task, error) {
	t, err := k.workboard.Heartbeat(id, agent, runID, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskHeartbeat, t, "heartbeat", map[string]any{"agent": agent, "run_id": runID})
	return t, nil
}

func (k *Kernel) CommentWorkboardTask(corr, id, author, body string) (workboard.Task, error) {
	t, err := k.workboard.Comment(id, author, body, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskCommented, t, "commented", map[string]any{"author": author})
	return t, nil
}

func (k *Kernel) BlockWorkboardTask(corr, id, actor, reason string) (workboard.Task, error) {
	t, err := k.workboard.Block(id, actor, reason, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "blocked", map[string]any{"actor": actor, "reason": reason})
	return t, nil
}

func (k *Kernel) FailWorkboardTask(corr, id, actor, reason string) (workboard.Task, workboard.RetryDecision, error) {
	t, decision, err := k.workboard.Fail(id, actor, reason, time.Now())
	if err != nil {
		return workboard.Task{}, workboard.RetryDecision{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, decision.Action, map[string]any{
		"actor":         actor,
		"reason":        reason,
		"failure_count": decision.FailureCount,
		"max_attempts":  decision.MaxAttempts,
		"next_attempt":  decision.NextAttempt,
		"retry":         decision.Retry,
		"exhausted":     decision.Exhausted,
		"escalate_to":   decision.EscalateTo,
	})
	return t, decision, nil
}

func (k *Kernel) UnblockWorkboardTask(corr, id, actor string) (workboard.Task, error) {
	t, err := k.workboard.Unblock(id, actor, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "unblocked", map[string]any{"actor": actor})
	return t, nil
}

func (k *Kernel) SetWorkboardRetryPolicy(corr, id, actor string, policy *workboard.RetryPolicy) (workboard.Task, error) {
	t, err := k.workboard.SetRetryPolicy(id, actor, policy, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	extra := map[string]any{"actor": actor}
	if policy != nil {
		extra["max_attempts"] = policy.MaxAttempts
		extra["escalate_to"] = policy.EscalateTo
	} else {
		extra["cleared"] = true
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "retry_policy", extra)
	return t, nil
}

func (k *Kernel) CompleteWorkboardTask(corr, id, actor string) (workboard.Task, error) {
	t, err := k.workboard.Complete(id, actor, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "completed", map[string]any{"actor": actor})
	// An ungated task completing normally still rolls up into any OKR that links it.
	k.recomputeOKRForTask(corr, id)
	return t, nil
}

func (k *Kernel) ReviewWorkboardTask(corr, id, actor, summary string) (workboard.Task, error) {
	t, err := k.workboard.Review(id, actor, summary, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "review", map[string]any{"actor": actor})
	return t, nil
}

func (k *Kernel) ArchiveWorkboardTask(corr, id, actor string) (workboard.Task, error) {
	t, err := k.workboard.Archive(id, actor, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "archived", map[string]any{"actor": actor})
	return t, nil
}

func (k *Kernel) LinkWorkboardTask(corr, id, typ, target string) (workboard.Task, error) {
	t, err := k.workboard.Link(id, typ, target, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskLinked, t, "linked", map[string]any{"type": typ, "target": target})
	return t, nil
}

func (k *Kernel) AddWorkboardDependency(corr, id, dependsOn string) (workboard.Task, error) {
	t, err := k.workboard.AddDependency(id, dependsOn, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskDependency, t, "dependency_added", map[string]any{"depends_on": dependsOn})
	return t, nil
}

func (k *Kernel) ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (workboard.Task, error) {
	t, err := k.workboard.ReclaimStale(id, actor, staleAfter, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	action := "reclaimed"
	decision := workboard.RetryDecisionFor(t, "stale heartbeat")
	if decision.Exhausted {
		action = "escalate"
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, action, map[string]any{
		"actor":          actor,
		"stale_after_ms": staleAfter.Milliseconds(),
		"failure_count":  decision.FailureCount,
		"max_attempts":   decision.MaxAttempts,
		"next_attempt":   decision.NextAttempt,
		"retry":          decision.Retry,
		"exhausted":      decision.Exhausted,
		"escalate_to":    decision.EscalateTo,
	})
	return t, nil
}

func (k *Kernel) SweepStaleWorkboardClaims(corr, actor string, staleAfter time.Duration, limit int) ([]workboard.Task, error) {
	tasks, err := k.workboard.SweepStaleClaims(actor, staleAfter, limit, time.Now())
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		action := "reclaimed"
		decision := workboard.RetryDecisionFor(t, "stale heartbeat")
		if decision.Exhausted {
			action = "escalate"
		}
		k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, action, map[string]any{
			"actor":          actor,
			"stale_after_ms": staleAfter.Milliseconds(),
			"sweep":          true,
			"failure_count":  decision.FailureCount,
			"max_attempts":   decision.MaxAttempts,
			"next_attempt":   decision.NextAttempt,
			"retry":          decision.Retry,
			"exhausted":      decision.Exhausted,
			"escalate_to":    decision.EscalateTo,
		})
	}
	return tasks, nil
}

func (k *Kernel) publishWorkboard(corr string, kind event.Kind, t workboard.Task, action string, extra map[string]any) {
	payload := map[string]any{
		"id":       t.ID,
		"title":    t.Title,
		"status":   string(t.Status),
		"priority": t.Priority,
		"assignee": t.Assignee,
		"tenant":   t.Tenant,
		"action":   action,
	}
	for key, val := range extra {
		if val != "" {
			payload[key] = val
		}
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "workboard." + t.ID,
		Kind:          kind,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// assureCriteriaMaxTokens bounds the per-criterion verdict reply. It is larger
// than the plain completion check because the judge itemizes each criterion.
const assureCriteriaMaxTokens = 800

// ProveTask closes the proof loop for a task: it runs the acceptance-criteria
// judge over the task's answer, gathers the artifacts + hash-chained journal
// range produced under corr as evidence, records the resulting proof, and
// publishes a proved/unproven event. When the proof is satisfied the task
// reaches done; otherwise it parks in review with the gap visible. A task with
// no acceptance criteria is judged on overall completion alone.
//
// answer is the run output to judge; pass "" to have ProveTask synthesize a
// proxy from the task's latest attempt summary and recent comments (used by the
// manual `agt workboard prove` path, which has no answer in hand).
func (k *Kernel) ProveTask(ctx context.Context, corr, id, answer string) (workboard.Task, error) {
	if k.workboard == nil {
		return workboard.Task{}, workboard.ErrNotFound
	}
	t, ok := k.workboard.Get(id)
	if !ok {
		return workboard.Task{}, workboard.ErrNotFound
	}
	if strings.TrimSpace(answer) == "" {
		answer = taskAnswerProxy(t)
	}
	ev := k.gatherProofEvidence(corrOrClaim(t, corr))
	taskText := t.Title
	if d := strings.TrimSpace(t.Description); d != "" {
		taskText += "\n\n" + d
	}
	verdict, judged, err := k.verifyCriteria(ctx, ev.Corr, taskText, answer, t.Criteria)
	if err != nil {
		return workboard.Task{}, err
	}
	p := proof.Proof{
		Verdict:  verdict,
		Criteria: judged,
		Evidence: ev,
		Attempts: len(t.Attempts),
		Judge:    "verify",
		ProvedMS: time.Now().UnixMilli(),
	}
	proved, err := k.workboard.Prove(id, "assure", p, time.Now())
	if err != nil {
		return workboard.Task{}, err
	}
	kind, action := event.KindWorkboardTaskProved, "proved"
	if proved.Proof == nil || !proved.Proof.Satisfied() {
		kind, action = event.KindWorkboardTaskUnproven, "unproven"
	}
	k.publishWorkboard(corr, kind, proved, action, map[string]any{
		"complete":  verdict.Complete,
		"gap":       verdict.Gap,
		"criteria":  len(proved.Criteria),
		"unmet":     p.UnmetCount(),
		"artifacts": len(ev.Artifacts),
	})
	// Roll the proof up into any OKR key results that link this task.
	k.recomputeOKRForTask(corr, id)
	return proved, nil
}

// verifyCriteria judges each acceptance criterion against the answer, returning
// the overall verdict plus per-criterion outcomes. With no criteria it falls
// back to the plain completion judge (verifyCompletion) and returns no criteria.
func (k *Kernel) verifyCriteria(ctx context.Context, corr, task, answer string, criteria []proof.Criterion) (assure.Verdict, []proof.Criterion, error) {
	if len(criteria) == 0 {
		v, err := k.verifyCompletion(ctx, corr, task, answer)
		return v, nil, err
	}
	var cb strings.Builder
	for i, c := range criteria {
		fmt.Fprintf(&cb, "%d. %s\n", i+1, c.Text)
	}
	prompt := "You are a strict acceptance-criteria checker. Given a TASK, the ANSWER an agent produced, and a numbered list of ACCEPTANCE CRITERIA, decide for EACH criterion whether the answer actually satisfies it. Be skeptical: a plan or a promise to do it is NOT satisfaction.\n\n" +
		"Reply with ONLY a JSON object and no other text:\n" +
		"{\"complete\": true|false, \"gap\": \"<what is still missing overall; empty string if everything is satisfied>\", \"criteria\": [{\"text\": \"<the criterion, verbatim>\", \"met\": true|false, \"note\": \"<short reason>\"}]}\n" +
		"Set \"complete\" to true only if EVERY criterion is met.\n\n" +
		"TASK:\n" + task + "\n\nACCEPTANCE CRITERIA:\n" + cb.String() + "\nANSWER:\n" + answer
	resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
		Model:         k.Model(),
		CorrelationID: corr,
		TaskType:      "verify",
		MaxTokens:     assureCriteriaMaxTokens,
		Messages:      []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	})
	if err != nil {
		return assure.Verdict{}, nil, err
	}
	verdict, judged := parseCriteriaVerdict(resp.Message.Content)
	// Mirror verifyCompletion: journal the verdict under corr so `agt why` shows
	// why the proof gate opened or held.
	if k.bus != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject:       "agent.agent-" + corr + ".assure",
			Kind:          event.KindAssureVerdict,
			Actor:         "assure",
			CorrelationID: corr,
			Payload:       map[string]any{"complete": verdict.Complete, "gap": verdict.Gap, "criteria": len(judged)},
		})
	}
	return verdict, judged, nil
}

// parseCriteriaVerdict extracts the extended verdict {complete,gap,criteria[]}
// from a model reply, tolerating a ```json fence or surrounding prose. An
// unparseable reply becomes "not complete" so the gate holds rather than
// declaring a false success.
func parseCriteriaVerdict(reply string) (assure.Verdict, []proof.Criterion) {
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return assure.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}, nil
	}
	var raw struct {
		Complete bool   `json:"complete"`
		Gap      string `json:"gap"`
		Criteria []struct {
			Text string `json:"text"`
			Met  bool   `json:"met"`
			Note string `json:"note"`
		} `json:"criteria"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &raw); err != nil {
		return assure.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}, nil
	}
	judged := make([]proof.Criterion, 0, len(raw.Criteria))
	for _, c := range raw.Criteria {
		judged = append(judged, proof.Criterion{Text: c.Text, Met: c.Met, Note: c.Note})
	}
	return assure.Verdict{Complete: raw.Complete, Gap: raw.Gap}, judged
}

// gatherProofEvidence collects the checkable evidence a proof rests on: the
// artifact index entries and the hash-chained journal sequence range produced
// under corr. It never fails — missing evidence just yields an emptier record.
func (k *Kernel) gatherProofEvidence(corr string) proof.Evidence {
	ev := proof.Evidence{Corr: corr}
	if strings.TrimSpace(corr) == "" {
		return ev
	}
	if ix := k.ArtifactIndex(); ix != nil {
		for _, e := range ix.List(artifact.Filter{Corr: corr}) {
			ev.Artifacts = append(ev.Artifacts, e.ID)
		}
	}
	if j := k.Journal(); j != nil {
		// Scan by correlation id, not a fixed tail window: under a busy fleet the
		// task's events can scroll out of the newest-N, which would silently record
		// no journal range for a proof whose whole point is after-the-fact checkability.
		_ = j.Range(func(e *event.Event) error {
			if e == nil || e.CorrelationID != corr {
				return nil
			}
			if ev.JournalFrom == 0 || e.Seq < ev.JournalFrom {
				ev.JournalFrom = e.Seq
			}
			if e.Seq > ev.JournalTo {
				ev.JournalTo = e.Seq
			}
			return nil
		})
	}
	return ev
}

// corrOrClaim resolves the correlation id the task's work ran under: the caller
// hint if given, else the task's claim run id, else the most recent attempt's
// run id.
func corrOrClaim(t workboard.Task, corr string) string {
	if c := strings.TrimSpace(corr); c != "" {
		return c
	}
	if t.Claim != nil && strings.TrimSpace(t.Claim.RunID) != "" {
		return t.Claim.RunID
	}
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if r := strings.TrimSpace(t.Attempts[i].RunID); r != "" {
			return r
		}
	}
	return ""
}

// taskAnswerProxy synthesizes an "answer" to judge when no run output is in hand
// (the manual prove path), drawing on the latest attempt summary and the most
// recent comments.
func taskAnswerProxy(t workboard.Task) string {
	var b strings.Builder
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(t.Attempts[i].Summary); s != "" {
			b.WriteString("Latest attempt: ")
			b.WriteString(s)
			b.WriteByte('\n')
			break
		}
	}
	start := len(t.Comments) - 3
	if start < 0 {
		start = 0
	}
	for _, c := range t.Comments[start:] {
		if s := strings.TrimSpace(c.Body); s != "" {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	if b.Len() == 0 {
		return "(no run output recorded for this task)"
	}
	return b.String()
}
