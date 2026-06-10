// SPDX-License-Identifier: MIT

package toolforge

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Deterministic, strictly-increasing clock: same-millisecond Adds would
	// otherwise flip List's creation-time sort onto the ID tiebreaker.
	tick := int64(0)
	s.now = func() time.Time {
		tick++
		return time.UnixMilli(1_700_000_000_000 + tick)
	}
	return s
}

func draft(t *testing.T, s *Store, name string) ScriptTool {
	t.Helper()
	st, err := s.Add(ScriptTool{
		Name:        name,
		Description: "echoes its input",
		Language:    "python",
		Code:        "print(open('stdin.txt').read())",
	})
	if err != nil {
		t.Fatalf("Add(%s): %v", name, err)
	}
	return st
}

// TestLifecycle_TestedCodeGoesLive walks the governed pipeline end to end:
// draft (never offered) → promote refused while untested → passing test →
// promote → active (offered) → quarantine (pulled) → re-promote without a
// re-test (the code didn't change).
func TestLifecycle_TestedCodeGoesLive(t *testing.T) {
	s := openStore(t)
	st := draft(t, s, "echo")
	if st.Status != StatusDraft || st.TestedOK {
		t.Fatalf("new draft = %s/%v, want draft/untested", st.Status, st.TestedOK)
	}
	if len(s.Active()) != 0 {
		t.Fatal("a draft must never be offered")
	}

	if _, err := s.Promote(st.ID); !errors.Is(err, ErrUntested) {
		t.Fatalf("promote untested: got %v, want ErrUntested", err)
	}

	if _, err := s.RecordTest(st.ID, true); err != nil {
		t.Fatalf("RecordTest: %v", err)
	}
	got, err := s.Promote(st.ID)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if got.Status != StatusActive {
		t.Fatalf("status = %s, want active", got.Status)
	}
	if act := s.Active(); len(act) != 1 || act[0].Name != "echo" {
		t.Fatalf("Active = %v, want [echo]", act)
	}

	if _, err := s.Quarantine(st.ID); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	if len(s.Active()) != 0 {
		t.Fatal("a quarantined tool must not be offered")
	}
	// Un-edited code keeps its passing test → direct re-promote works.
	if got, err = s.Promote(st.Name); err != nil || got.Status != StatusActive {
		t.Fatalf("re-promote = %s/%v, want active/nil", got.Status, err)
	}
}

// TestUpdate_CodeChangeDemotes: editing the code of an ACTIVE tool pulls it
// back to draft and clears the test record — only tested code is ever live.
// Editing only the description demotes nothing.
func TestUpdate_CodeChangeDemotes(t *testing.T) {
	s := openStore(t)
	st := draft(t, s, "echo")
	if _, err := s.RecordTest(st.ID, true); err != nil {
		t.Fatalf("RecordTest: %v", err)
	}
	if _, err := s.Promote(st.ID); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// A docs-only edit keeps the tool live and tested.
	got, err := s.Update(st.ID, func(dst *ScriptTool) { dst.Description = "better words" })
	if err != nil {
		t.Fatalf("Update(desc): %v", err)
	}
	if got.Status != StatusActive || !got.TestedOK {
		t.Fatalf("desc edit demoted: %s/%v", got.Status, got.TestedOK)
	}

	// A code edit demotes + clears the test record.
	got, err = s.Update(st.ID, func(dst *ScriptTool) { dst.Code = "print('v2')" })
	if err != nil {
		t.Fatalf("Update(code): %v", err)
	}
	if got.Status != StatusDraft || got.TestedOK || got.TestedMS != 0 {
		t.Fatalf("code edit = %s/%v/%d, want draft/untested/0", got.Status, got.TestedOK, got.TestedMS)
	}
	if len(s.Active()) != 0 {
		t.Fatal("edited tool still offered")
	}
	if _, err := s.Promote(st.ID); !errors.Is(err, ErrUntested) {
		t.Fatalf("promote after edit: got %v, want ErrUntested", err)
	}
}

// TestUpdate_ProtectsIdentity: a hostile mutator cannot rename a tool,
// resurrect it, or forge a test record.
func TestUpdate_ProtectsIdentity(t *testing.T) {
	s := openStore(t)
	st := draft(t, s, "echo")
	got, err := s.Update(st.ID, func(dst *ScriptTool) {
		dst.ID = "evil"
		dst.Name = "other"
		dst.Status = StatusActive
		dst.TestedOK = true
		dst.CreatedMS = 1
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.ID != st.ID || got.Name != "echo" || got.Status != StatusDraft || got.TestedOK || got.CreatedMS != st.CreatedMS {
		t.Fatalf("identity/lifecycle not protected: %+v", got)
	}
}

func TestValidate(t *testing.T) {
	base := ScriptTool{Name: "ok_tool", Description: "d", Language: "python", Code: "print(1)"}
	if err := Validate(base); err != nil {
		t.Fatalf("valid tool rejected: %v", err)
	}
	cases := []struct {
		label  string
		mutate func(*ScriptTool)
	}{
		{"bad name (dash)", func(st *ScriptTool) { st.Name = "bad-name" }},
		{"bad name (upper)", func(st *ScriptTool) { st.Name = "Bad" }},
		{"bad name (leading digit)", func(st *ScriptTool) { st.Name = "1tool" }},
		{"empty description", func(st *ScriptTool) { st.Description = "  " }},
		{"empty language", func(st *ScriptTool) { st.Language = "" }},
		{"empty code", func(st *ScriptTool) { st.Code = " " }},
		{"oversized code", func(st *ScriptTool) { st.Code = strings.Repeat("x", maxCodeBytes+1) }},
		{"schema not an object", func(st *ScriptTool) { st.InputSchema = `["nope"]` }},
		{"schema not JSON", func(st *ScriptTool) { st.InputSchema = `{nope` }},
	}
	for _, tc := range cases {
		st := base
		tc.mutate(&st)
		if err := Validate(st); err == nil {
			t.Errorf("%s: accepted", tc.label)
		}
	}
}

func TestAdd_NameUnique(t *testing.T) {
	s := openStore(t)
	draft(t, s, "echo")
	if _, err := s.Add(ScriptTool{Name: "echo", Description: "d", Language: "node", Code: "x"}); err == nil {
		t.Fatal("duplicate name accepted")
	}
}

func TestQuarantine_OnlyActive(t *testing.T) {
	s := openStore(t)
	st := draft(t, s, "echo")
	if _, err := s.Quarantine(st.ID); err == nil {
		t.Fatal("quarantining a draft accepted (drafts are not live)")
	}
}

// TestPersistence_RoundTrip: records survive a reopen, addressed by id or name.
func TestPersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st, err := s.Add(ScriptTool{Name: "echo", Description: "d", Language: "python", Code: "print(1)", InputSchema: `{"type":"object"}`})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.RecordTest("echo", true); err != nil { // by name
		t.Fatalf("RecordTest: %v", err)
	}

	re, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, found := re.Get(st.ID)
	if !found || !got.TestedOK || got.InputSchema != `{"type":"object"}` {
		t.Fatalf("reloaded = %+v / %v", got, found)
	}
	if _, found := re.Get("echo"); !found {
		t.Fatal("lookup by name failed after reopen")
	}
	if filepath.Base(re.path) != "scripttools.json" {
		t.Fatalf("unexpected store file %s", re.path)
	}
}

func TestRemove(t *testing.T) {
	s := openStore(t)
	st := draft(t, s, "echo")
	gone, ok, err := s.Remove(st.Name)
	if err != nil || !ok || gone.ID != st.ID {
		t.Fatalf("Remove = %+v/%v/%v", gone, ok, err)
	}
	if _, ok, _ := s.Remove(st.Name); ok {
		t.Fatal("second remove reported existing")
	}
	if s.Count() != 0 {
		t.Fatalf("Count = %d, want 0", s.Count())
	}
}
