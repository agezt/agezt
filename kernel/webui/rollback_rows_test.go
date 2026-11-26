// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The console's rollback answers are the CONTRACT's rows, byte for byte.
//
// The rows moved from the platform's store into the contract
// (kernel/contract/rollbackapi) and the console serves them. These tests pin
// the BYTES: a struct that changed its member order, or a field that appeared
// out of nowhere, would move the wire — and the wire is what the browser
// reads.

func TestTheConsoleServesTheContractRollbackListBytes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{
		{ID: "rb-a", Kind: rollbackCheckpointKindSkill, Action: "skill.set", RunID: "run-a",
			SubjectID: "s-1", SubjectName: "ops", BeforeStatus: "paused", CreatedMS: 100},
	}}); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	s, _ := newServer(t, &fakeCaller{}, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodGet,
		"/api/rollback/checkpoints?token=secret&run_id=run-a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"checkpoints":[{"id":"rb-a","kind":"skill.status","action":"skill.set","run_id":"run-a",` +
		`"subject_id":"s-1","subject_name":"ops","before_status":"paused","created_ms":100}],"count":1,` +
		`"run_id":"run-a"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the list answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}

func TestTheConsoleServesTheContractRollbackApplyBytes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{
		{ID: "rb-a", Kind: rollbackCheckpointKindSkill, Action: "skill.set", RunID: "run-a",
			SubjectID: "s-1", SubjectName: "ops", BeforeStatus: "paused", CreatedMS: 100, AppliedMS: 500},
	}}); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	s, _ := newServer(t, &fakeCaller{}, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodPost,
		"/api/rollback/apply?token=secret", `{"id":"rb-a"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"applied":false,"checkpoint":{"id":"rb-a","kind":"skill.status","action":"skill.set",` +
		`"run_id":"run-a","subject_id":"s-1","subject_name":"ops","before_status":"paused",` +
		`"created_ms":100,"applied_ms":500},"reason":"already applied"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the apply answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}

func TestTheConsoleServesTheContractRollbackAppliedMembers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{
		{ID: "rb-a", Kind: rollbackCheckpointKindSkill, Action: "skill.set", RunID: "run-a",
			SubjectID: "s-1", SubjectName: "ops", BeforeStatus: "paused", CreatedMS: 100},
	}}); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodPost,
		"/api/rollback/apply?token=secret", `{"id":"rb-a"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	members := make([]string, 0, len(got))
	for name := range got {
		members = append(members, name)
	}
	sort.Strings(members)
	want := []string{"applied", "checkpoint", "result"}
	if strings.Join(members, ",") != strings.Join(want, ",") {
		t.Fatalf("the applied answer's members = %v want %v", members, want)
	}
	if got["applied"] != true {
		t.Fatalf("the applied answer's applied = %v want true", got["applied"])
	}
}
