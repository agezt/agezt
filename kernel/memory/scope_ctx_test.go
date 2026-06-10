// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestTool_RecallDefaultsToCtxScope: a run carrying a per-agent scope (M786)
// recalls that scope's private notes + shared memory without naming itself;
// an explicit scope param still wins; an unscoped ctx sees shared only.
func TestTool_RecallDefaultsToCtxScope(t *testing.T) {
	m, _ := newTestManager(t)
	if _, _, err := m.Remember("", RememberSpec{Subject: "target", Content: "shared-fact"}); err != nil {
		t.Fatalf("remember shared: %v", err)
	}
	if _, _, err := m.Remember("", RememberSpec{
		Subject: "target notes", Content: "researcher-private-fact",
		Tags: map[string]string{"scope": "researcher"},
	}); err != nil {
		t.Fatalf("remember private: %v", err)
	}
	if _, _, err := m.Remember("", RememberSpec{
		Subject: "target notes", Content: "ops-private-fact",
		Tags: map[string]string{"scope": "ops"},
	}); err != nil {
		t.Fatalf("remember ops: %v", err)
	}
	tool := m.Tool()

	recall := func(ctx context.Context, input string) string {
		t.Helper()
		res, err := tool.Invoke(ctx, json.RawMessage(input))
		if err != nil {
			t.Fatalf("invoke: %v", err)
		}
		return res.Output
	}

	// ctx scope = researcher → shared + researcher, never ops.
	out := recall(WithScope(context.Background(), "researcher"),
		`{"action":"recall","query":"target"}`)
	if !strings.Contains(out, "shared-fact") || !strings.Contains(out, "researcher-private-fact") {
		t.Errorf("scoped ctx recall missing shared/private: %q", out)
	}
	if strings.Contains(out, "ops-private-fact") {
		t.Errorf("scoped ctx recall leaked another scope: %q", out)
	}

	// Explicit param wins over the ctx default.
	out = recall(WithScope(context.Background(), "researcher"),
		`{"action":"recall","query":"target","scope":"ops"}`)
	if !strings.Contains(out, "ops-private-fact") || strings.Contains(out, "researcher-private-fact") {
		t.Errorf("explicit scope param should win: %q", out)
	}

	// Unscoped ctx: shared only (the pre-M786 behaviour).
	out = recall(context.Background(), `{"action":"recall","query":"target"}`)
	if !strings.Contains(out, "shared-fact") ||
		strings.Contains(out, "researcher-private-fact") || strings.Contains(out, "ops-private-fact") {
		t.Errorf("unscoped recall must see shared only: %q", out)
	}
}

// TestTool_RememberStaysSharedByDefault: ctx scope changes what an agent READS,
// not what it writes — a remember without an explicit scope param stays shared
// ("shared brain, private notes": private writes are opt-in via the param).
func TestTool_RememberStaysSharedByDefault(t *testing.T) {
	m, _ := newTestManager(t)
	tool := m.Tool()
	ctx := WithScope(context.Background(), "researcher")
	if _, err := tool.Invoke(ctx, json.RawMessage(
		`{"action":"remember","subject":"finding","content":"useful-for-everyone"}`)); err != nil {
		t.Fatalf("remember: %v", err)
	}
	// Visible to an UNscoped recall → it was written shared.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"action":"recall","query":"finding"}`))
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if !strings.Contains(res.Output, "useful-for-everyone") {
		t.Errorf("agent write under a scoped ctx should stay shared: %q", res.Output)
	}
}
