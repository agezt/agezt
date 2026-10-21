// SPDX-License-Identifier: MIT

package runs

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// collectEvent builds one journal event for the fold. The sequence number
// mirrors the timestamp, so it is unique per event and assertable.
func collectEvent(kind event.Kind, corr string, ts int64, payload map[string]any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{
		Kind: kind, CorrelationID: corr, Seq: ts, TSUnixMS: ts, Payload: raw,
	}
}

// collectJournal replays the events through the fold's journal port.
func collectJournal(events ...*event.Event) func(func(*event.Event) error) error {
	return func(visit func(*event.Event) error) error {
		for _, e := range events {
			if err := visit(e); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestCollect_Received(t *testing.T) {
	rows, err := Collect(collectJournal(collectEvent(event.KindTaskReceived, "c1", 100, map[string]any{
		"intent": "do the thing", "agent": "ops",
	})))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	row, ok := rows["c1"]
	if !ok {
		t.Fatalf("no row for the correlation: %v", rows)
	}
	if row.CorrelationID != "c1" || row.Intent != "do the thing" || row.Agent != "ops" {
		t.Errorf("row = %+v", row)
	}
	if row.StartedUnixMS != 100 {
		t.Errorf("started = %d, want the event's time", row.StartedUnixMS)
	}
	if row.StartedSeq != 100 {
		t.Errorf("started seq = %d, want the event's sequence", row.StartedSeq)
	}
	if row.Phase != "starting" {
		t.Errorf("phase = %q, want starting", row.Phase)
	}
	if row.Completed || row.Failed || row.Abandoned {
		t.Errorf("row = %+v, want a running run", row)
	}
}

func TestCollect_Completed(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindTaskReceived, "c1", 100, map[string]any{"intent": "i"}),
		collectEvent(event.KindTaskCompleted, "c1", 500, map[string]any{
			"iters": 3, "answer": "the answer",
		}),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	row := rows["c1"]
	if !row.Completed || row.CompletedUnixMS != 500 {
		t.Errorf("row = %+v, want the completed run", row)
	}
	if row.Iters != 3 {
		t.Errorf("iters = %d, want 3", row.Iters)
	}
	if row.AnswerPreview != "the answer" {
		t.Errorf("answer = %q", row.AnswerPreview)
	}
}

// A completion without a reception still records what it has, so the
// operator at least sees the chain id.
func TestCollect_CompletedWithoutReceived(t *testing.T) {
	rows, err := Collect(collectJournal(collectEvent(event.KindTaskCompleted, "c9", 500, nil)))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	row, ok := rows["c9"]
	if !ok || !row.Completed || row.CorrelationID != "c9" {
		t.Errorf("rows = %v, want the half we have", rows)
	}
}

func TestCollect_Failed(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindTaskReceived, "c1", 100, nil),
		collectEvent(event.KindTaskFailed, "c1", 700, map[string]any{"reason": "max_iters"}),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	row := rows["c1"]
	if !row.Failed || row.FailedUnixMS != 700 || row.FailReason != "max_iters" {
		t.Errorf("row = %+v, want the failed run", row)
	}
	if row.Status() != "failed" {
		t.Errorf("status = %q, want failed", row.Status())
	}
}

func TestCollect_Abandoned(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindTaskReceived, "c1", 100, nil),
		collectEvent(event.KindTaskAbandoned, "c1", 900, nil),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !rows["c1"].Abandoned {
		t.Errorf("row = %+v, want the abandoned run", rows["c1"])
	}
	if rows["c1"].Status() != "abandoned" {
		t.Errorf("status = %q, want abandoned", rows["c1"].Status())
	}
}

func TestCollect_SpawnLinksTheChildToItsParent(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindTaskReceived, "parent", 100, nil),
		collectEvent(event.KindSubAgentSpawned, "parent", 110, map[string]any{
			"child_correlation": "child", "parent": "parent",
		}),
		collectEvent(event.KindTaskReceived, "child", 120, nil),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if rows["child"].ParentCorrelation != "parent" {
		t.Errorf("child = %+v, want the parent link", rows["child"])
	}
}

// A spawn without a parent names no lead, so it links nothing.
func TestCollect_SpawnWithoutAParentLinksNothing(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindSubAgentSpawned, "parent", 110, map[string]any{
			"child_correlation": "child",
		}),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}

func TestCollect_SpendAndModel(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindTaskReceived, "c1", 100, nil),
		collectEvent(event.KindBudgetConsumed, "c1", 200, map[string]any{
			"cost_microcents": 12, "model": "model-a",
		}),
		collectEvent(event.KindBudgetConsumed, "c1", 300, map[string]any{
			"cost_microcents": 5, "model": "model-b",
		}),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	row := rows["c1"]
	if row.SpentMicrocents != 17 {
		t.Errorf("spent = %d, want the sum of the two spends", row.SpentMicrocents)
	}
	// The model is first-wins: a later fallback does not overwrite it.
	if row.Model != "model-a" {
		t.Errorf("model = %q, want the first model", row.Model)
	}
}

// A budget event for an unknown correlation must not conjure a phantom run.
func TestCollect_SpendWithoutARunIsIgnored(t *testing.T) {
	rows, err := Collect(collectJournal(collectEvent(event.KindBudgetConsumed, "ghost", 200, map[string]any{
		"cost_microcents": 12,
	})))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}

// A budget event without a correlation is ignored outright.
func TestCollect_SpendWithoutACorrelationIsIgnored(t *testing.T) {
	rows, err := Collect(collectJournal(collectEvent(event.KindBudgetConsumed, "", 200, map[string]any{
		"cost_microcents": 12,
	})))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}

func TestCollect_Phase(t *testing.T) {
	rows, err := Collect(collectJournal(
		collectEvent(event.KindTaskReceived, "c1", 100, nil),
		collectEvent(event.KindLLMRequest, "c1", 110, nil),
		collectEvent(event.KindToolInvoked, "c1", 120, map[string]any{"tool": "search"}),
		collectEvent(event.KindToolResult, "c1", 130, map[string]any{"tool": "search"}),
		collectEvent(event.KindTaskContinued, "c1", 140, nil),
	))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	row := rows["c1"]
	if row.Phase != "continuing" {
		t.Errorf("phase = %q, want the last phase", row.Phase)
	}
	if row.Tool != "search" {
		t.Errorf("tool = %q, want the tool in flight", row.Tool)
	}
}

// A lifecycle event for an unknown correlation must not conjure a phantom run.
func TestCollect_PhaseWithoutARunIsIgnored(t *testing.T) {
	rows, err := Collect(collectJournal(collectEvent(event.KindLLMRequest, "ghost", 110, nil)))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}

// Each activity phase is the run's current phase while it lasts.
func TestCollect_Phases(t *testing.T) {
	cases := []struct {
		name  string
		kind  event.Kind
		tool  string
		phase string
	}{
		{"thinking", event.KindLLMRequest, "", "thinking"},
		{"using tool", event.KindToolInvoked, "search", "using tool"},
		{"observing tool", event.KindToolResult, "search", "observing tool"},
		{"continuing", event.KindTaskContinued, "", "continuing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{}
			if tc.tool != "" {
				payload["tool"] = tc.tool
			}
			rows, err := Collect(collectJournal(
				collectEvent(event.KindTaskReceived, "c1", 100, nil),
				collectEvent(tc.kind, "c1", 110, payload),
			))
			if err != nil {
				t.Fatalf("collect: %v", err)
			}
			row := rows["c1"]
			if row.Phase != tc.phase {
				t.Errorf("phase = %q, want %q", row.Phase, tc.phase)
			}
			if tc.tool != "" && row.Tool != tc.tool {
				t.Errorf("tool = %q, want %q", row.Tool, tc.tool)
			}
		})
	}
}

func TestCollect_Extraction(t *testing.T) {
	if got := extractIntent(nil); got != "" {
		t.Errorf("intent = %q, want empty", got)
	}
	if got := extractIntent([]byte("not json")); got != "" {
		t.Errorf("intent = %q, want empty", got)
	}
	if got := extractIntent([]byte(`{"intent":"i"}`)); got != "i" {
		t.Errorf("intent = %q, want i", got)
	}
	if got := extractAgent([]byte(`{"agent":"ops"}`)); got != "ops" {
		t.Errorf("agent = %q, want ops", got)
	}
	if got := extractTool([]byte(`{"tool":"search"}`)); got != "search" {
		t.Errorf("tool = %q, want search", got)
	}
	if got := extractIters([]byte(`{"iters":4}`)); got != 4 {
		t.Errorf("iters = %d, want 4", got)
	}
	if got := extractIters([]byte("not json")); got != 0 {
		t.Errorf("iters = %d, want 0", got)
	}
	if got := extractCostMicrocents([]byte(`{"cost_microcents":9}`)); got != 9 {
		t.Errorf("cost = %d, want 9", got)
	}
	if got := extractModel([]byte(`{"model":"m"}`)); got != "m" {
		t.Errorf("model = %q, want m", got)
	}
	if got := extractReason([]byte(`{"reason":"timeout"}`)); got != "timeout" {
		t.Errorf("reason = %q, want timeout", got)
	}
	child, parent := extractSpawnLink([]byte(`{"child_correlation":"c","parent":"p"}`))
	if child != "c" || parent != "p" {
		t.Errorf("spawn = %q/%q, want c/p", child, parent)
	}
	if c, p := extractSpawnLink([]byte("not json")); c != "" || p != "" {
		t.Errorf("spawn = %q/%q, want empty", c, p)
	}
}

// The answer excerpt is one line and bounded.
func TestCollect_AnswerPreview(t *testing.T) {
	if got := extractAnswerPreview([]byte(`{"answer":"  hello \n world  "}`)); got != "hello world" {
		t.Errorf("answer = %q, want the collapsed line", got)
	}
	if got := extractAnswerPreview([]byte(`{"answer":"   "}`)); got != "" {
		t.Errorf("answer = %q, want empty", got)
	}
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	raw, _ := json.Marshal(map[string]any{"answer": string(long)})
	got := extractAnswerPreview(raw)
	if len([]rune(got)) != answerPreviewRunes+1 {
		t.Errorf("answer length = %d, want the excerpt and the ellipsis", len([]rune(got)))
	}
}

func TestRunRows(t *testing.T) {
	rows := map[string]Run{
		"c1": {CorrelationID: "c1", Intent: "i", Model: "m", Agent: "ops", SpentMicrocents: 7, StartedUnixMS: 100, ParentCorrelation: "p", Completed: true},
	}
	out := RunRows(rows)
	if len(out) != 1 {
		t.Fatalf("rows = %v, want one", out)
	}
	r := out[0]
	if r.CorrelationID != "c1" || r.Intent != "i" || r.Model != "m" || r.Agent != "ops" {
		t.Errorf("row = %+v", r)
	}
	if r.SpentMicrocents != 7 || r.StartedUnixMS != 100 || r.ParentCorrelation != "p" || !r.Completed {
		t.Errorf("row = %+v", r)
	}
}
