// SPDX-License-Identifier: MIT

package workboard

// The board's PROOF CYCLE: the judgment that decides whether a task actually
// did what it promised.
//
// The cycle used to live in the daemon. These tests pin what the move kept:
// the board REFUSES without a store (the contract's row), the board GATHERS
// the evidence under the right correlation id (the rule, not a tail window),
// the board JUDGES each criterion and HOLDS the gate when the answer falls
// short, the board PUBLISHES the verdict on the host's bus, and the board
// TELLS the host that a task was proved (the objective rollup is the host's
// business).
//
// The host keeps the judge, the artifacts and the journal. The board keeps the
// rules — and these tests are what say so.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

// fakeArtifacts is a host that answers "which artifacts were recorded".
type fakeArtifacts struct {
	asked []remoteapi.Filter
	rows  []remoteapi.Artifact
}

func (f *fakeArtifacts) List(filter remoteapi.Filter) []remoteapi.Artifact {
	f.asked = append(f.asked, filter)
	return f.rows
}

// fakeJournal is a host that lets the board walk its durable events.
type fakeJournal struct {
	events []*event.Event
}

func (f *fakeJournal) Range(visit func(*event.Event) error) error {
	for _, e := range f.events {
		if err := visit(e); err != nil {
			return err
		}
	}
	return nil
}

// judge is a host that answers the per-criterion question.
type judge struct {
	reply string
	err   error
	asked []llm.CompletionRequest
}

func (j *judge) complete(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	j.asked = append(j.asked, req)
	if j.err != nil {
		return nil, j.err
	}
	return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: j.reply}}, nil
}

// The board refuses to prove without a store.
//
// The refusal is the CONTRACT's row, the same one every other board verb
// gives: a daemon with no board answers no board.
func TestTheBoardRefusesToProveWithoutAStore(t *testing.T) {
	svc := NewService(nil, api.BoardDependencies{})
	if _, err := svc.Prove(context.Background(), "corr", "id", "answer"); err != workboardapi.ErrNoStore {
		t.Errorf("Prove without a store: err = %v, want the contract's refusal", err)
	}
}

// The board gathers no evidence when the host has none.
//
// A host with no artifact index and no journal gives an EMPTIER record, never
// a failed one: the proof still rests on the answer, and the missing evidence
// is simply absent.
func TestTheBoardGathersNoEvidenceWhenTheHostHasNone(t *testing.T) {
	store := openBoard(t)
	svc := NewService(store, api.BoardDependencies{
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proved, err := svc.Prove(context.Background(), "corr-1", task.ID, "an answer")
	if err != nil {
		t.Fatalf("Prove without evidence: %v", err)
	}
	if len(proved.Proof.Evidence.Artifacts) != 0 {
		t.Errorf("the evidence = %v, want none — the host has no artifact index", proved.Proof.Evidence.Artifacts)
	}
	if proved.Proof.Evidence.JournalFrom != 0 || proved.Proof.Evidence.JournalTo != 0 {
		t.Errorf("the journal range = %d..%d, want none — the host has no journal", proved.Proof.Evidence.JournalFrom, proved.Proof.Evidence.JournalTo)
	}
	if !proved.Proof.Satisfied() {
		t.Error("the proof must still be satisfied — missing evidence is not a failed proof")
	}
}

// The board proves a task that has no criteria with the host's plain judge.
//
// A task with no acceptance criteria has no criteria to judge, so the host's
// own completion judge answers whether the answer accomplished the task at
// all. The board asks; it never judges completion itself.
func TestTheBoardProvesACriterialessTaskWithTheHostsJudge(t *testing.T) {
	var asked []string
	store := openBoard(t)
	svc := NewService(store, api.BoardDependencies{
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			asked = append(asked, corr+"|"+task+"|"+answer)
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proved, err := svc.Prove(context.Background(), "corr-1", task.ID, "the answer")
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if len(asked) != 1 {
		t.Fatalf("the host's judge was asked %d times, want 1", len(asked))
	}
	if !strings.Contains(asked[0], "the answer") {
		t.Errorf("the judge was asked %q, want the answer the caller gave", asked[0])
	}
	if proved.Proof == nil {
		t.Fatal("the task must carry the proof")
	}
	if !proved.Proof.Satisfied() {
		t.Errorf("the proof = %+v, want it satisfied — the host's judge said complete", proved.Proof)
	}
}

// The board HOLDS the gate when the answer falls short.
//
// A proof that cannot be reached is not a proof that passed: the verdict
// carries the gap, and the proof is recorded as unsatisfied.
func TestTheBoardHoldsTheGateWhenTheAnswerFallsShort(t *testing.T) {
	store := openBoard(t)
	svc := NewService(store, api.BoardDependencies{
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return assureapi.Verdict{Complete: false, Gap: "the report is missing"}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proved, err := svc.Prove(context.Background(), "corr", task.ID, "a plan")
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if proved.Proof == nil {
		t.Fatal("the task must carry the proof")
	}
	if proved.Proof.Satisfied() {
		t.Error("the proof must not be satisfied when the judge said it is not")
	}
	if proved.Proof.Verdict.Gap != "the report is missing" {
		t.Errorf("the gap = %q, want the judge's own gap", proved.Proof.Verdict.Gap)
	}
}

// The board judges EACH criterion and reads the reply.
//
// The verdict says whether EVERY criterion is met, and the per-criterion
// outcomes are recorded verbatim — a criterion the judge never saw is a
// criterion that was never checked.
func TestTheBoardJudgesEachCriterionAndReadsTheReply(t *testing.T) {
	store := openBoard(t)
	j := &judge{reply: `{"complete": false, "gap": "one is missing", "criteria": [{"text": "first", "met": true, "note": "done"}, {"text": "second", "met": false, "note": "absent"}]}`}
	svc := NewService(store, api.BoardDependencies{
		Complete: j.complete,
		Model:    func() string { return "the-model" },
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{
		Title:              "promised",
		AcceptanceCriteria: []string{"first", "second"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proved, err := svc.Prove(context.Background(), "corr", task.ID, "an answer")
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if len(j.asked) != 1 {
		t.Fatalf("the judge was asked %d times, want 1", len(j.asked))
	}
	if j.asked[0].Model != "the-model" {
		t.Errorf("the judge was asked with model %q, want the host's model", j.asked[0].Model)
	}
	if !strings.Contains(j.asked[0].Messages[0].Content, "first") ||
		!strings.Contains(j.asked[0].Messages[0].Content, "second") {
		t.Error("the judge must be asked with EVERY criterion")
	}
	if len(proved.Proof.Criteria) != 2 {
		t.Fatalf("the proof carries %d criteria, want 2", len(proved.Proof.Criteria))
	}
	if !proved.Proof.Criteria[0].Met {
		t.Error("the first criterion was met — the proof must say so")
	}
	if proved.Proof.Criteria[1].Met {
		t.Error("the second criterion was not met — the proof must say so")
	}
	if proved.Proof.Satisfied() {
		t.Error("the proof must not be satisfied when one criterion is unmet")
	}
}

// The board HOLDS the gate when the reply cannot be read.
//
// An unreadable reply is not a pass: the gate holds and the gap says why.
func TestTheBoardHoldsTheGateWhenTheReplyCannotBeRead(t *testing.T) {
	store := openBoard(t)
	svc := NewService(store, api.BoardDependencies{
		Complete: (&judge{reply: "I am a teapot"}).complete,
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{
		Title:              "promised",
		AcceptanceCriteria: []string{"first"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proved, err := svc.Prove(context.Background(), "corr", task.ID, "an answer")
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if proved.Proof.Satisfied() {
		t.Error("an unreadable reply must not satisfy the gate")
	}
	if !strings.Contains(proved.Proof.Verdict.Gap, "not valid JSON") {
		t.Errorf("the gap = %q, want it to say the reply was not JSON", proved.Proof.Verdict.Gap)
	}
}

// The board HOLDS the gate when the host has no judge.
//
// A task whose criteria cannot be judged is not a task that passed. The board
// answers with an error and records nothing — a proof that cannot be reached
// is not a proof that succeeded.
func TestTheBoardHoldsTheGateWhenTheHostHasNoJudge(t *testing.T) {
	store := openBoard(t)
	svc := NewService(store, api.BoardDependencies{})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{
		Title:              "promised",
		AcceptanceCriteria: []string{"first"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Prove(context.Background(), "corr", task.ID, "an answer"); err == nil {
		t.Fatal("a task that cannot be judged must not be proved")
	}

	// The criterialess task needs the host's plain judge, and without it the
	// gate holds the same way.
	task2, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "plain"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Prove(context.Background(), "corr", task2.ID, "an answer"); err == nil {
		t.Fatal("a task that cannot be judged must not be proved")
	}
}

// The board gathers the evidence under the RIGHT correlation id.
//
// The rule is the board's: the evidence belongs to the correlation id the
// task's work ran under, and a task with no work under any correlation id
// rests on no evidence at all.
func TestTheBoardGathersTheEvidenceUnderTheRightCorrelationId(t *testing.T) {
	store := openBoard(t)
	artifacts := &fakeArtifacts{rows: []remoteapi.Artifact{{ID: "artifact-1"}}}
	journal := &fakeJournal{events: []*event.Event{
		{CorrelationID: "corr-1", Seq: 7},
		{CorrelationID: "other", Seq: 9},
		{CorrelationID: "corr-1", Seq: 11},
	}}
	svc := NewService(store, api.BoardDependencies{
		Artifacts: artifacts,
		Journal:   journal,
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	proved, err := svc.Prove(context.Background(), "corr-1", task.ID, "an answer")
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if len(artifacts.asked) != 1 {
		t.Fatalf("the host was asked for artifacts %d times, want 1", len(artifacts.asked))
	}
	if artifacts.asked[0].Corr != "corr-1" {
		t.Errorf("the artifacts were asked for corr %q, want the caller's corr", artifacts.asked[0].Corr)
	}
	if proved.Proof.Evidence.Corr != "corr-1" {
		t.Errorf("the evidence names corr %q, want the correlation the work ran under", proved.Proof.Evidence.Corr)
	}
	if len(proved.Proof.Evidence.Artifacts) != 1 || proved.Proof.Evidence.Artifacts[0] != "artifact-1" {
		t.Errorf("the evidence = %v, want the host's artifact", proved.Proof.Evidence.Artifacts)
	}
	if proved.Proof.Evidence.JournalFrom != 7 || proved.Proof.Evidence.JournalTo != 11 {
		t.Errorf("the journal range = %d..%d, want 7..11 — only the task's own events", proved.Proof.Evidence.JournalFrom, proved.Proof.Evidence.JournalTo)
	}
}

// The board publishes the verdict on the host's bus.
//
// The subject and the kind are the board's own; the bus is the host's.
func TestTheBoardPublishesTheVerdict(t *testing.T) {
	store := openBoard(t)
	var published []event.Spec
	svc := NewService(store, api.BoardDependencies{
		Publish: func(spec event.Spec) { published = append(published, spec) },
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Prove(context.Background(), "corr", task.ID, "an answer"); err != nil {
		t.Fatalf("Prove: %v", err)
	}
	var found bool
	for _, spec := range published {
		if spec.Kind == event.KindWorkboardTaskProved {
			found = true
			if spec.Subject != "workboard."+task.ID {
				t.Errorf("the subject = %q, want the board's own", spec.Subject)
			}
		}
	}
	if !found {
		t.Fatal("the board must announce the task it proved")
	}
}

// The board tells the host that a task was proved.
//
// The objective rollup is the host's business: a proved task counts toward a
// Key Result, and the board holds no knowledge of objectives.
func TestTheBoardTellsTheHostThatATaskWasProved(t *testing.T) {
	store := openBoard(t)
	var proved []string
	svc := NewService(store, api.BoardDependencies{
		TaskProved: func(corr, taskID string) { proved = append(proved, corr+"|"+taskID) },
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Prove(context.Background(), "corr-1", task.ID, "an answer"); err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if len(proved) != 1 || proved[0] != "corr-1|"+task.ID {
		t.Errorf("the host was told %v, want the task that was proved", proved)
	}
}

// The board judges the answer the caller gave, and falls back to the task's
// own record when there is none.
//
// The manual prove path has no run output in hand, so the board synthesizes
// one from the task's latest attempt and its most recent comments.
func TestTheBoardSynthesizesAnAnswerWhenThereIsNone(t *testing.T) {
	store := openBoard(t)
	var judged string
	svc := NewService(store, api.BoardDependencies{
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			judged = answer
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Prove(context.Background(), "corr", task.ID, "   "); err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if judged == "" {
		t.Fatal("the board must judge something")
	}
	if !strings.Contains(judged, "no run output recorded") {
		t.Errorf("the synthesized answer = %q, want the board's own stand-in", judged)
	}
}

// The board refuses a task it cannot find.
//
// An unknown task is not a task that passed.
func TestTheBoardRefusesATaskItCannotFind(t *testing.T) {
	svc := NewService(openBoard(t), api.BoardDependencies{})
	if _, err := svc.Prove(context.Background(), "corr", "no-such-task", "an answer"); err != workboardapi.ErrNotFound {
		t.Errorf("Prove of an unknown task: err = %v, want the contract's not-found", err)
	}
}

// The board is quiet without a bus.
//
// The task still moved and the caller still holds the answer: the board
// publishes through a port, and a daemon with no bus simply has no port.
func TestTheProofIsQuietWithoutABus(t *testing.T) {
	store := openBoard(t)
	svc := NewService(store, api.BoardDependencies{
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return assureapi.Verdict{Complete: true}, nil
		},
	})
	task, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "promised"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Prove(context.Background(), "corr", task.ID, "an answer"); err != nil {
		t.Fatalf("Prove without a bus: %v", err)
	}
}

// The reply the judge sends is read even when it is fenced in prose.
//
// A judge that wraps its JSON in a fence still said what it meant.
func TestTheJudgeReplyIsReadEvenWhenItIsFenced(t *testing.T) {
	reply := "Here is my verdict:\n```json\n{\"complete\": true, \"gap\": \"\", \"criteria\": [{\"text\": \"first\", \"met\": true}]}\n```\n"
	verdict, judged := parseCriteriaVerdict(reply)
	if !verdict.Complete {
		t.Errorf("the verdict = %+v, want it complete", verdict)
	}
	if len(judged) != 1 || !judged[0].Met {
		t.Errorf("the judged criteria = %+v, want the fenced criterion", judged)
	}
}

// The reply the judge sends survives the wire.
//
// The verdict crosses the bus as JSON, so what the board read must survive
// the trip.
func TestTheVerdictSurvivesTheWire(t *testing.T) {
	verdict := assureapi.Verdict{Complete: true, Gap: ""}
	raw, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back assureapi.Verdict
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Complete != verdict.Complete {
		t.Errorf("the verdict did not survive the wire: %+v", back)
	}
}

// The board's own refusal is not the contract's row.
//
// The judge refusal is the BOARD's: the host may not name the module, and the
// module may not name the host, so a refusal only the module gives needs no
// shared row — but it must not be mistaken for the store's refusal.
func TestTheJudgesRefusalIsNotTheStoresRefusal(t *testing.T) {
	if errors.Is(errNoJudge, workboardapi.ErrNoStore) {
		t.Error("the judge's refusal must not be the store's refusal")
	}
	if !strings.Contains(errNoJudge.Error(), "judge") {
		t.Errorf("the refusal says %q, want it to name the judge", errNoJudge.Error())
	}
}
