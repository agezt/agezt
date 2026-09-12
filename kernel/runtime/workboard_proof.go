// SPDX-License-Identifier: MIT

// Runtime workboard: proof-verification (ProveTask + verifyCriteria + parseCriteriaVerdict + gatherProofEvidence).
// Code extracted from workboard.go during the Day-121 god-file split.
// Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/workboard"
)

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
		v, err := k.VerifyCompletion(ctx, corr, task, answer)
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
	resp, err := k.completeAux(ctx, corr, "verify", agent.CompletionRequest{
		Model:     k.Model(),
		MaxTokens: assureCriteriaMaxTokens,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: prompt}},
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
