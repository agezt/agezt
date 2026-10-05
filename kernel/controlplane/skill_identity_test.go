// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"testing"
)

func skillIdentityFixture(t *testing.T) (*runtime.Kernel, *Server, string) {
	t.Helper()
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if _, err := k.AddProfile(roster.Profile{Slug: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: "fixture", Body: "body", Agent: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.Forge().Promote("seed", sk.ID); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, sk.ID
}
func TestSkillNativeMutationsShareOrderedOperationIdentity(t *testing.T) {
	kinds := map[string]event.Kind{CmdSkillPromote: event.KindSkillPromoted, CmdSkillQuarantine: event.KindSkillQuarantined, CmdSkillArchive: event.KindSkillReverted, CmdSkillRevert: event.KindSkillReverted, CmdSkillRestore: event.KindSkillRestored, CmdSkillShare: event.KindSkillShared, CmdSkillReassign: event.KindSkillReassigned, CmdSkillImport: event.KindSkillCreated}
	for cmd, kind := range kinds {
		t.Run(cmd, func(t *testing.T) {
			k, s, id := skillIdentityFixture(t)
			before := int64(-1)
			if err := k.Journal().Range(func(e *event.Event) error { before = e.Seq; return nil }); err != nil {
				t.Fatal(err)
			}
			r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": id, "reason": "owned", "status": "draft", "agent": "reviewer", "name": "imported", "body": "new body", "correlation_id": "untrusted-input"}})[0]
			if r.Type != RespResult {
				t.Fatal(r)
			}
			var invoked, completed *event.Event
			invocations, completions := 0, 0
			var domain []*event.Event
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Seq <= before {
					return nil
				}
				copy := *e
				if e.Subject == "op."+cmd {
					if e.Kind == event.KindOpInvoked {
						invoked = &copy
						invocations++
					}
					if e.Kind == event.KindOpCompleted {
						completed = &copy
						completions++
					}
				}
				if e.Kind == kind {
					domain = append(domain, &copy)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if invoked == nil || completed == nil || len(domain) != 1 || invocations != 1 || completions != 1 {
				t.Fatalf("missing arc invoked=%v domain=%v completed=%v", invoked, domain, completed)
			}
			if invoked.CorrelationID == "" || invoked.CorrelationID == "untrusted-input" || domain[0].CorrelationID != invoked.CorrelationID || completed.CorrelationID != invoked.CorrelationID || !(invoked.Seq < domain[0].Seq && domain[0].Seq < completed.Seq) {
				t.Fatalf("EXPECTED: one trusted nonempty ordered operation/domain identity; ACTUAL: cmd=%s audit=%q domain=%q terminal=%q seq=%d/%d/%d", cmd, invoked.CorrelationID, domain[0].CorrelationID, completed.CorrelationID, invoked.Seq, domain[0].Seq, completed.Seq)
			}
		})
	}
}
func TestSkillOwnershipHistoryIncludesSharedAndReassigned(t *testing.T) {
	k, s, id := skillIdentityFixture(t)
	for _, cmd := range []string{CmdSkillShare, CmdSkillReassign} {
		r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": id, "agent": "reviewer"}})[0]
		if r.Type != RespResult {
			t.Fatal(r)
		}
	}
	want := map[string]*event.Event{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindSkillShared || e.Kind == event.KindSkillReassigned {
			var p map[string]any
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p["id"] == id {
				copy := *e
				want[string(e.Kind)] = &copy
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(want) != 2 {
		t.Fatalf("fixture missing ownership events=%v", want)
	}
	r := callAppHost(t, s, Request{ID: "history", Cmd: CmdSkillHistory, Token: "primary", Args: map[string]any{"id": id}})[0]
	if r.Type != RespResult {
		t.Fatal(r)
	}
	rows, ok := r.Result["events"].([]any)
	if !ok {
		t.Fatalf("history rows=%v", r.Result)
	}
	seen := map[string]bool{}
	last := float64(-1)
	for _, raw := range rows {
		row := raw.(map[string]any)
		seq := row["seq"].(float64)
		if seq <= last {
			t.Fatal("history order changed")
		}
		last = seq
		kind := row["kind"].(string)
		if expected := want[kind]; expected != nil {
			if row["id"] != expected.ID || row["correlation_id"] != expected.CorrelationID {
				t.Fatalf("ownership provenance=%v", row)
			}
			seen[kind] = true
		}
	}
	if len(seen) != 2 {
		t.Fatalf("EXPECTED: both journaled ownership changes visible in skill history; ACTUAL: seen=%v history=%v", seen, r.Result)
	}
}
