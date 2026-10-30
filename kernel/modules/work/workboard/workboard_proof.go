// SPDX-License-Identifier: MIT

package workboard

// The board's PROOF CYCLE: the durable, checkable evidence that a task
// actually satisfied its acceptance criteria.
//
// The cycle used to live in the daemon, which spelled out the whole judgment:
// the evidence to gather, the criterion prompt, the reply to parse, and the
// verdict to publish. The board keeps the task and the proof that closes it,
// so the board keeps the cycle.
//
// What the host keeps: the judge (the model), the artifacts, the journal, the
// bus and the objective library. What the board keeps: the RULES — which
// correlation id the evidence belongs to, what each criterion is judged
// against, how a reply is read, and what "the gate holds" means.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/proofapi"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
)

// assureCriteriaMaxTokens bounds the per-criterion verdict reply. It is larger
// than the plain completion check because the judge itemizes each criterion.
const assureCriteriaMaxTokens = 800

// Prove judges an answer against the task's acceptance criteria and records
// the proof on the task.
//
// The proof is the BOARD's own: the task carries the proof, and the proof
// carries the verdict.
func (s *service) Prove(ctx context.Context, corr, id, answer string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, ok := store.Get(id)
	if !ok {
		return workboardapi.Task{}, workboardapi.ErrNotFound
	}
	if strings.TrimSpace(answer) == "" {
		answer = taskAnswerProxy(t)
	}
	ev := s.gatherProofEvidence(corrOrClaim(t, corr))
	taskText := t.Title
	if d := strings.TrimSpace(t.Description); d != "" {
		taskText += "\n\n" + d
	}
	verdict, judged, err := s.verifyCriteria(ctx, ev.Corr, taskText, answer, t.Criteria)
	if err != nil {
		return workboardapi.Task{}, err
	}
	p := proofapi.Proof{
		Verdict:  verdict,
		Criteria: judged,
		Evidence: ev,
		Attempts: len(t.Attempts),
		Judge:    "verify",
		ProvedMS: time.Now().UnixMilli(),
	}
	proved, err := store.Prove(id, "assure", p, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	kind, action := event.KindWorkboardTaskProved, "proved"
	if proved.Proof == nil || !proved.Proof.Satisfied() {
		kind, action = event.KindWorkboardTaskUnproven, "unproven"
	}
	s.publish(corr, kind, proved, action, map[string]any{
		"complete":  verdict.Complete,
		"gap":       verdict.Gap,
		"criteria":  len(proved.Criteria),
		"unmet":     p.UnmetCount(),
		"artifacts": len(ev.Artifacts),
	})
	// Roll the proof up into any OKR key results that link this task.
	if s.deps.TaskProved != nil {
		s.deps.TaskProved(corr, id)
	}
	return proved, nil
}

// verifyCriteria judges each acceptance criterion against the answer, returning
// the overall verdict plus per-criterion outcomes. With no criteria it falls
// back to the plain completion judge (the host's) and returns no criteria.
func (s *service) verifyCriteria(ctx context.Context, corr, task, answer string, criteria []proofapi.Criterion) (assureapi.Verdict, []proofapi.Criterion, error) {
	if len(criteria) == 0 {
		if s.deps.VerifyCompletion == nil {
			return assureapi.Verdict{}, nil, errNoJudge
		}
		v, err := s.deps.VerifyCompletion(ctx, corr, task, answer)
		return v, nil, err
	}
	if s.deps.Complete == nil {
		return assureapi.Verdict{}, nil, errNoJudge
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
	model := ""
	if s.deps.Model != nil {
		model = s.deps.Model()
	}
	resp, err := s.deps.Complete(ctx, corr, "verify", llm.CompletionRequest{
		Model:     model,
		MaxTokens: assureCriteriaMaxTokens,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	})
	if err != nil {
		return assureapi.Verdict{}, nil, err
	}
	verdict, judged := parseCriteriaVerdict(resp.Message.Content)
	// Mirror the host's completion judge: journal the verdict under corr so
	// `agt why` shows why the proof gate opened or held.
	if s.deps.Publish != nil {
		s.deps.Publish(event.Spec{
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
func parseCriteriaVerdict(reply string) (assureapi.Verdict, []proofapi.Criterion) {
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return assureapi.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}, nil
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
		return assureapi.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}, nil
	}
	judged := make([]proofapi.Criterion, 0, len(raw.Criteria))
	for _, c := range raw.Criteria {
		judged = append(judged, proofapi.Criterion{Text: c.Text, Met: c.Met, Note: c.Note})
	}
	return assureapi.Verdict{Complete: raw.Complete, Gap: raw.Gap}, judged
}

// gatherProofEvidence collects the checkable evidence a proof rests on: the
// artifact index entries and the hash-chained journal sequence range produced
// under corr. It never fails — missing evidence just yields an emptier record.
func (s *service) gatherProofEvidence(corr string) proofapi.Evidence {
	ev := proofapi.Evidence{Corr: corr}
	if strings.TrimSpace(corr) == "" {
		return ev
	}
	if s.deps.Artifacts != nil {
		for _, e := range s.deps.Artifacts.List(remoteapi.Filter{Corr: corr}) {
			ev.Artifacts = append(ev.Artifacts, e.ID)
		}
	}
	if s.deps.Journal != nil {
		// Scan by correlation id, not a fixed tail window: under a busy fleet the
		// task's events can scroll out of the newest-N, which would silently record
		// no journal range for a proof whose whole point is after-the-fact checkability.
		_ = s.deps.Journal.Range(func(e *event.Event) error {
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
func corrOrClaim(t workboardapi.Task, corr string) string {
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
func taskAnswerProxy(t workboardapi.Task) string {
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

// errNoJudge is the board's answer when the host has no judge to ask.
//
// The gate HOLDS: a task whose criteria cannot be judged is not a task that
// passed, and a proof that cannot be reached is not a proof that succeeded.
var errNoJudge = errors.New("workboard: the host has no completion judge to prove with")
