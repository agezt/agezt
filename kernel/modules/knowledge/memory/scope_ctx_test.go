// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
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

// TestTool_RememberDefaultsToCtxScope (M915): each agent keeps its own memory —
// a remember without an explicit scope param lands in the run's agent scope,
// invisible to the shared view and to other agents. Sharing is the explicit,
// selective opt-in (shared=true), and an unscoped run still writes shared.
func TestTool_RememberDefaultsToCtxScope(t *testing.T) {
	m, _ := newTestManager(t)
	tool := m.Tool()
	ctx := WithScope(context.Background(), "researcher")
	if _, err := tool.Invoke(ctx, json.RawMessage(
		`{"action":"remember","subject":"finding","content":"researcher-only-note"}`)); err != nil {
		t.Fatalf("remember: %v", err)
	}

	// Invisible to an UNscoped recall → it was written private.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"action":"recall","query":"finding"}`))
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if strings.Contains(res.Output, "researcher-only-note") {
		t.Errorf("agent write must default to its own scope, not shared: %q", res.Output)
	}
	// Invisible to another agent's recall.
	res, _ = tool.Invoke(WithScope(context.Background(), "ops"), json.RawMessage(`{"action":"recall","query":"finding"}`))
	if strings.Contains(res.Output, "researcher-only-note") {
		t.Errorf("another agent must not see the private note: %q", res.Output)
	}
	// Visible to the writer's own recall.
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"action":"recall","query":"finding"}`))
	if !strings.Contains(res.Output, "researcher-only-note") {
		t.Errorf("the writer should recall its own private note: %q", res.Output)
	}

	// shared=true is the selective opt-in to the shared brain.
	if _, err := tool.Invoke(ctx, json.RawMessage(
		`{"action":"remember","subject":"decision","content":"useful-for-everyone","shared":true}`)); err != nil {
		t.Fatalf("remember shared: %v", err)
	}
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"action":"recall","query":"decision"}`))
	if !strings.Contains(res.Output, "useful-for-everyone") {
		t.Errorf("shared=true write should land in shared memory: %q", res.Output)
	}

	// scope:"shared" — the sentinel a model plausibly produces — means shared too.
	if _, err := tool.Invoke(ctx, json.RawMessage(
		`{"action":"remember","subject":"law","content":"sentinel-shared-note","scope":"shared"}`)); err != nil {
		t.Fatalf("remember scope=shared: %v", err)
	}
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"action":"recall","query":"law"}`))
	if !strings.Contains(res.Output, "sentinel-shared-note") {
		t.Errorf("scope \"shared\" should mean the shared brain: %q", res.Output)
	}

	// An explicit scope param still wins over the ctx default.
	if _, err := tool.Invoke(ctx, json.RawMessage(
		`{"action":"remember","subject":"handoff","content":"note-for-ops","scope":"ops"}`)); err != nil {
		t.Fatalf("remember explicit scope: %v", err)
	}
	res, _ = tool.Invoke(WithScope(context.Background(), "ops"), json.RawMessage(`{"action":"recall","query":"handoff"}`))
	if !strings.Contains(res.Output, "note-for-ops") {
		t.Errorf("explicit scope param should win over ctx scope: %q", res.Output)
	}

	// An unscoped run (no agent identity) keeps writing shared, as before.
	if _, err := tool.Invoke(context.Background(), json.RawMessage(
		`{"action":"remember","subject":"plain","content":"unscoped-run-note"}`)); err != nil {
		t.Fatalf("remember unscoped: %v", err)
	}
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"action":"recall","query":"plain"}`))
	if !strings.Contains(res.Output, "unscoped-run-note") {
		t.Errorf("an unscoped run's write should stay shared: %q", res.Output)
	}
}

// TestScopedID_NoCrossAgentFlip (M915): two agents privately noting the same
// content yield two records — the second write must not reinforce the first and
// flip its scope tag, which would hide the note from its original author.
func TestScopedID_NoCrossAgentFlip(t *testing.T) {
	m, _ := newTestManager(t)
	tool := m.Tool()
	body := `{"action":"remember","subject":"build","content":"the build needs Go 1.22"}`
	if _, err := tool.Invoke(WithScope(context.Background(), "researcher"), json.RawMessage(body)); err != nil {
		t.Fatalf("researcher remember: %v", err)
	}
	if _, err := tool.Invoke(WithScope(context.Background(), "ops"), json.RawMessage(body)); err != nil {
		t.Fatalf("ops remember: %v", err)
	}
	all, _ := m.All()
	if len(all) != 2 {
		t.Fatalf("same content in two scopes should make two records, got %d", len(all))
	}
	// The first author still recalls its own copy.
	res, _ := tool.Invoke(WithScope(context.Background(), "researcher"), json.RawMessage(`{"action":"recall","query":"build"}`))
	if !strings.Contains(res.Output, "Go 1.22") {
		t.Errorf("researcher lost its note to the ops write: %q", res.Output)
	}
	// Same agent re-stating the same fact still dedupes (reinforce, not duplicate).
	if _, err := tool.Invoke(WithScope(context.Background(), "ops"), json.RawMessage(body)); err != nil {
		t.Fatalf("ops re-remember: %v", err)
	}
	if all, _ = m.All(); len(all) != 2 {
		t.Errorf("same-agent re-write must reinforce, not duplicate: got %d records", len(all))
	}
}

// TestPromote (M915): promoting a private record clears its scope so the shared
// view recalls it; the act is journaled (memory.promoted) and idempotent.
func TestPromote(t *testing.T) {
	m, j := newTestManager(t)
	rec, _, err := m.Remember("corr", RememberSpec{
		Subject: "deploy", Content: "project deploy needs the staging key first",
		Tags: map[string]string{"source": "agent", "scope": "ops"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if subs := recallSubjects(t, m, "project", ""); subs["deploy"] {
		t.Fatal("private record visible to the shared view before promote")
	}

	promoted, found, err := m.Promote("corr", rec.ID)
	if err != nil || !found {
		t.Fatalf("Promote: found=%v err=%v", found, err)
	}
	if scopeOf(promoted.Tags) != "" {
		t.Errorf("promoted record still scoped: %v", promoted.Tags)
	}
	if subs := recallSubjects(t, m, "project", ""); !subs["deploy"] {
		t.Error("promoted record should surface in the shared view")
	}
	if n := countKind(t, j, event.KindMemoryPromoted); n != 1 {
		t.Errorf("memory.promoted events = %d, want 1", n)
	}

	// Idempotent: promoting an already-shared record is found but journals nothing new.
	if _, found, err := m.Promote("corr", rec.ID); err != nil || !found {
		t.Fatalf("re-Promote: found=%v err=%v", found, err)
	}
	if n := countKind(t, j, event.KindMemoryPromoted); n != 1 {
		t.Errorf("re-promote must not journal again, events = %d", n)
	}
	// Unknown id reports not-found.
	if _, found, err := m.Promote("corr", "no-such-id"); err != nil || found {
		t.Fatalf("Promote(unknown): found=%v err=%v", found, err)
	}
}

// TestDistillTagsRunScope (M915): a named agent's distilled facts stay its
// private notes — the run ctx carries the agent scope and distillation tags
// facts with it, so per-run distillation doesn't flood the shared brain.
func TestDistillTagsRunScope(t *testing.T) {
	m, _ := newTestManager(t)
	prov := fakeDistiller{body: `{"facts":[{"subject":"repo","content":"it is a go monorepo","type":"FACT"}]}`}
	ctx := WithScope(context.Background(), "researcher")
	ids, err := m.Distill(ctx, "run-s", prov, "model", "intent", "transcript")
	if err != nil || len(ids) != 1 {
		t.Fatalf("distill: ids=%d err=%v", len(ids), err)
	}
	rec, ok, _ := m.Get(ids[0])
	if !ok {
		t.Fatal("distilled record not found")
	}
	if rec.Tags["scope"] != "researcher" {
		t.Errorf("distilled fact should carry the run's agent scope, tags=%v", rec.Tags)
	}
	// An unscoped run distills shared, as before.
	ids, err = m.Distill(context.Background(), "run-u", prov, "model", "intent", "transcript")
	if err != nil || len(ids) != 1 {
		t.Fatalf("unscoped distill: ids=%d err=%v", len(ids), err)
	}
	rec, _, _ = m.Get(ids[0])
	if rec.Tags["scope"] != "" {
		t.Errorf("unscoped distill must stay shared, tags=%v", rec.Tags)
	}
}
