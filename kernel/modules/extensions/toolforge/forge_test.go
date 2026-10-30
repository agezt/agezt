// SPDX-License-Identifier: MIT

package toolforge

// The script-tool FORGE SERVICE's own tests (W3.72).
//
// The daemon used to hold the forge's lifecycle — the store AND the
// operations over it, the sandbox test and the journaling. These tests pin
// the behaviour where it now lives: the module answers the lifecycle, the
// daemon keeps the names.
//
// The rule the lifecycle never breaks: only TESTED code goes live, any edit
// demotes the tool back to draft with its test record cleared, and the
// OPERATOR — not the agent — grants a promotion.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/event"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// The compile-time assertion: the service IS the module's face.
var _ extapi.ForgeService = (*Service)(nil)

// recorder collects the events the service publishes.
type recorder struct{ specs []event.Spec }

func (r *recorder) publish(spec event.Spec) { r.specs = append(r.specs, spec) }

// kinds returns the published event kinds, in order.
func (r *recorder) kinds() []string {
	out := make([]string, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, string(s.Kind))
	}
	return out
}

// stubRunner is a canned sandbox: it records the run and answers with a
// verdict.
type stubRunner struct {
	runs     int
	language string
	code     string
	input    string
	out      string
	isErr    bool
	err      error
}

func (s *stubRunner) RunScript(_ context.Context, language, code, inputJSON string) (string, bool, error) {
	s.runs++
	s.language, s.code, s.input = language, code, inputJSON
	return s.out, s.isErr, s.err
}

// newForge builds the service over a real catalogue (the module's own store)
// and the given seams.
func newForge(t *testing.T, rec *recorder, runner toolregapi.ScriptRunner) (*Service, *Store) {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open the forge store: %v", err)
	}
	deps := extapi.ForgeDependencies{Store: store, ScriptRunner: runner}
	if rec != nil {
		deps.Publish = rec.publish
	}
	return NewService(deps), store
}

// draftTool is the fixture every lifecycle test starts from.
func draftTool() toolforgeapi.ScriptTool {
	return toolforgeapi.ScriptTool{
		Name: "greet", Description: "greets a name", Language: "python", Code: "print('hi')",
	}
}

// TestTheForgeRefusesWithoutAStore: a service with no catalogue is a daemon
// with no script tools — the writes must SAY so rather than call into
// nothing.
func TestTheForgeRefusesWithoutAStore(t *testing.T) {
	svc := NewService(extapi.ForgeDependencies{})
	if _, err := svc.Draft("corr", draftTool()); !errors.Is(err, toolforgeapi.ErrNoStore) {
		t.Fatalf("draft without a store must refuse, got %v", err)
	}
	if _, _, err := svc.Update("corr", "greet", func(*toolforgeapi.ScriptTool) {}); !errors.Is(err, toolforgeapi.ErrNoStore) {
		t.Fatalf("update without a store must refuse, got %v", err)
	}
	if _, err := svc.Promote("corr", "greet"); !errors.Is(err, toolforgeapi.ErrNoStore) {
		t.Fatalf("promote without a store must refuse, got %v", err)
	}
	if _, err := svc.Quarantine("corr", "greet", "why"); !errors.Is(err, toolforgeapi.ErrNoStore) {
		t.Fatalf("quarantine without a store must refuse, got %v", err)
	}
	if _, err := svc.Remove("corr", "greet"); !errors.Is(err, toolforgeapi.ErrNoStore) {
		t.Fatalf("remove without a store must refuse, got %v", err)
	}
}

// TestDraftRegistersTheToolAndJournalsIt: the draft lands in the catalogue
// and the transition is published — the subject is the tool's own and the
// kind is the forge's.
func TestDraftRegistersTheToolAndJournalsIt(t *testing.T) {
	rec := &recorder{}
	svc, store := newForge(t, rec, nil)

	saved, err := svc.Draft("corr-1", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if saved.ID == "" || saved.Name != "greet" {
		t.Fatalf("the drafted tool is not the one that was added: %+v", saved)
	}
	if saved.Status != toolforgeapi.StatusDraft {
		t.Fatalf("a fresh tool must be a draft, got %q", saved.Status)
	}
	if rows := store.List(); len(rows) != 1 || rows[0].ID != saved.ID {
		t.Fatalf("the catalogue does not hold the drafted tool: %v", rows)
	}
	if got := rec.kinds(); len(got) != 1 || got[0] != string(event.KindScriptToolCreated) {
		t.Fatalf("the draft must be journaled as scripttool.created, got %v", got)
	}
	spec := rec.specs[0]
	if spec.Subject != "toolforge.greet" || spec.Actor != "toolforge" || spec.CorrelationID != "corr-1" {
		t.Fatalf("the journaled event is not the forge's own: %+v", spec)
	}
}

// TestTheEditDemotesTheToolAndClearsItsTest: only tested code goes live, so
// an edit takes the test record with it.
func TestTheEditDemotesTheToolAndClearsItsTest(t *testing.T) {
	rec := &recorder{}
	svc, _ := newForge(t, rec, nil)
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, err := svc.store.RecordTest(drafted.ID, true); err != nil {
		t.Fatalf("record the test: %v", err)
	}
	edited, ok, err := svc.Update("corr-2", drafted.ID, func(st *toolforgeapi.ScriptTool) {
		st.Code = "print('bye')"
	})
	if err != nil || !ok {
		t.Fatalf("Update: %v (ok=%v)", err, ok)
	}
	if edited.Status != toolforgeapi.StatusDraft {
		t.Fatalf("an edited tool must fall back to draft, got %q", edited.Status)
	}
	if edited.TestedOK {
		t.Fatal("the edit did not clear the test record")
	}
	if got := rec.kinds(); len(got) != 2 || got[1] != string(event.KindScriptToolUpdated) {
		t.Fatalf("the edit must be journaled as scripttool.updated, got %v", got)
	}
}

// TestTheEditOfAnUnknownToolIsNotAnError: the standing pattern — false and
// no error.
func TestTheEditOfAnUnknownToolIsNotAnError(t *testing.T) {
	svc, _ := newForge(t, nil, nil)
	if _, ok, err := svc.Update("corr", "ghost", func(*toolforgeapi.ScriptTool) {}); err != nil || ok {
		t.Fatalf("an unknown ref must answer false and no error, got ok=%v err=%v", ok, err)
	}
}

// TestTheTestRunsTheSandboxAndRecordsTheVerdict: the test is the gate —
// the tool's verdict is recorded and the transition is journaled.
func TestTheTestRunsTheSandboxAndRecordsTheVerdict(t *testing.T) {
	rec := &recorder{}
	runner := &stubRunner{out: "hello"}
	svc, _ := newForge(t, rec, runner)
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}

	rec2, out, err := svc.Test(context.Background(), "corr-2", drafted.ID, `{"name":"ersin"}`)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if out != "hello" {
		t.Fatalf("the sandbox output must come back verbatim, got %q", out)
	}
	if runner.runs != 1 || runner.language != "python" || runner.input != `{"name":"ersin"}` {
		t.Fatalf("the sandbox did not run the tool's code: %+v", runner)
	}
	if !rec2.TestedOK {
		t.Fatal("the passing test was not recorded")
	}
	if got := rec.kinds(); len(got) != 2 || got[1] != string(event.KindScriptToolTested) {
		t.Fatalf("the test must be journaled as scripttool.tested, got %v", got)
	}
}

// TestAFailingTestIsRecordedAsAFailure: the verdict is the sandbox's, and a
// failed test does not promote the tool.
func TestAFailingTestIsRecordedAsAFailure(t *testing.T) {
	runner := &stubRunner{out: "boom", isErr: true}
	svc, _ := newForge(t, nil, runner)
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	rec, _, err := svc.Test(context.Background(), "corr", drafted.ID, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if rec.TestedOK {
		t.Fatal("a failing sandbox run must not be recorded as a pass")
	}
	if runner.input != "{}" {
		t.Fatalf("an empty input must reach the script as {}, got %q", runner.input)
	}
}

// TestTheTestRefusesWithoutASandbox: the sandbox is the host's seam, and a
// missing one must refuse with a readable error instead of calling a nil
// func value.
func TestTheTestRefusesWithoutASandbox(t *testing.T) {
	svc, _ := newForge(t, nil, nil)
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	_, _, err = svc.Test(context.Background(), "corr", drafted.ID, "{}")
	if err == nil || !strings.Contains(err.Error(), "no sandbox runner") {
		t.Fatalf("the test must refuse with the sandbox's absence, got %v", err)
	}
}

// TestThePromotionAsksTheOperator: the agent asks, the OPERATOR decides.
func TestThePromotionAsksTheOperator(t *testing.T) {
	rec := &recorder{}
	runner := &stubRunner{out: "ok"}
	asked := 0
	svc, _ := newForge(t, rec, runner)
	svc.approve = func(_ context.Context, spec approvalapi.SubmitSpec) approvalapi.Outcome {
		asked++
		if spec.Capability != "toolforge.promote" {
			t.Fatalf("the request must ask for the promotion capability, got %q", spec.Capability)
		}
		return approvalapi.Outcome{Decision: approvalapi.DecisionGrant, Reason: "the operator said yes"}
	}
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, _, err := svc.Test(context.Background(), "corr", drafted.ID, "{}"); err != nil {
		t.Fatalf("Test: %v", err)
	}

	promoted, decision, reason, err := svc.RequestPromotion(context.Background(), "corr-2", drafted.ID)
	if err != nil {
		t.Fatalf("RequestPromotion: %v", err)
	}
	if asked != 1 {
		t.Fatalf("the promotion must reach the operator once, got %d", asked)
	}
	if decision != approvalapi.DecisionGrant {
		t.Fatalf("the operator's verdict must come back, got %q", decision)
	}
	if reason != "the operator said yes" {
		t.Fatalf("the operator's reason must come back, got %q", reason)
	}
	if promoted.Status != toolforgeapi.StatusActive {
		t.Fatalf("the granted tool must be live, got %q", promoted.Status)
	}
}

// TestADeniedPromotionIsAVerdictNotAFailure: a deny is the operator's
// answer, not an error — and the tool stays out of production.
func TestADeniedPromotionIsAVerdictNotAFailure(t *testing.T) {
	runner := &stubRunner{out: "ok"}
	svc, _ := newForge(t, nil, runner)
	svc.approve = func(context.Context, approvalapi.SubmitSpec) approvalapi.Outcome {
		return approvalapi.Outcome{Decision: approvalapi.DecisionDeny, Reason: "not now"}
	}
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, _, err := svc.Test(context.Background(), "corr", drafted.ID, "{}"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	st, decision, reason, err := svc.RequestPromotion(context.Background(), "corr", drafted.ID)
	if err != nil {
		t.Fatalf("a denied promotion must not be an error, got %v", err)
	}
	if decision != approvalapi.DecisionDeny || reason != "not now" {
		t.Fatalf("the operator's verdict must come back, got %q / %q", decision, reason)
	}
	if st.Status == toolforgeapi.StatusActive {
		t.Fatal("a denied tool must not go live")
	}
}

// TestAnUntestedToolNeverReachesTheOperator: the invariant is checked up
// front, so an untested draft never even reaches the queue.
func TestAnUntestedToolNeverReachesTheOperator(t *testing.T) {
	asked := 0
	svc, _ := newForge(t, nil, &stubRunner{out: "ok"})
	svc.approve = func(context.Context, approvalapi.SubmitSpec) approvalapi.Outcome {
		asked++
		return approvalapi.Outcome{Decision: approvalapi.DecisionGrant}
	}
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, _, _, err := svc.RequestPromotion(context.Background(), "corr", drafted.ID); !errors.Is(err, toolforgeapi.ErrUntested) {
		t.Fatalf("an untested tool must refuse, got %v", err)
	}
	if asked != 0 {
		t.Fatalf("the untested tool must not reach the operator, got %d requests", asked)
	}
}

// TestTheAutoPromotePromotesWithoutTheOperator: the host's switch.
func TestTheAutoPromotePromotesWithoutTheOperator(t *testing.T) {
	asked := 0
	runner := &stubRunner{out: "ok"}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open the forge store: %v", err)
	}
	svc := NewService(extapi.ForgeDependencies{Store: store, ScriptRunner: runner, AutoPromote: true})
	svc.approve = func(context.Context, approvalapi.SubmitSpec) approvalapi.Outcome {
		asked++
		return approvalapi.Outcome{Decision: approvalapi.DecisionGrant}
	}
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, _, err := svc.Test(context.Background(), "corr", drafted.ID, "{}"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	promoted, decision, reason, err := svc.RequestPromotion(context.Background(), "corr", drafted.ID)
	if err != nil {
		t.Fatalf("RequestPromotion: %v", err)
	}
	if asked != 0 {
		t.Fatalf("the auto-promote must not ask the operator, got %d requests", asked)
	}
	if promoted.Status != toolforgeapi.StatusActive {
		t.Fatalf("the auto-promoted tool must be live, got %q", promoted.Status)
	}
	if decision != approvalapi.DecisionGrant || reason != "auto-promote enabled" {
		t.Fatalf("the auto-promote must say so, got %q / %q", decision, reason)
	}
}

// TestTheQuarantineIsTheKillSwitch: the tool leaves production and the
// operator's reason is journaled.
func TestTheQuarantineIsTheKillSwitch(t *testing.T) {
	rec := &recorder{}
	runner := &stubRunner{out: "ok"}
	svc, _ := newForge(t, rec, runner)
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, _, err := svc.Test(context.Background(), "corr", drafted.ID, "{}"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if _, err := svc.Promote("corr", drafted.ID); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	quarantined, err := svc.Quarantine("corr-2", drafted.ID, "  it misbehaved  ")
	if err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	if quarantined.Status != toolforgeapi.StatusQuarantined {
		t.Fatalf("the quarantined tool must be out of production, got %q", quarantined.Status)
	}
	if got := rec.kinds(); len(got) != 4 || got[3] != string(event.KindScriptToolQuarantined) {
		t.Fatalf("the quarantine must be journaled as scripttool.quarantined, got %v", got)
	}
	payload, _ := rec.specs[3].Payload.(map[string]any)
	if payload["reason"] != "it misbehaved" {
		t.Fatalf("the operator's reason must be journaled, got %+v", payload)
	}
}

// TestTheRemoveTakesTheRowAndJournalsIt: the removal is journaled when the
// row was there — and the answer says whether it was.
func TestTheRemoveTakesTheRowAndJournalsIt(t *testing.T) {
	rec := &recorder{}
	svc, store := newForge(t, rec, nil)
	drafted, err := svc.Draft("corr", draftTool())
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	removed, err := svc.Remove("corr-2", drafted.ID)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatal("the remove must report that the row was there")
	}
	if rows := store.List(); len(rows) != 0 {
		t.Fatalf("the row is still in the catalogue: %v", rows)
	}
	if got := rec.kinds(); len(got) != 2 || got[1] != string(event.KindScriptToolRemoved) {
		t.Fatalf("the remove must be journaled as scripttool.removed, got %v", got)
	}
}
