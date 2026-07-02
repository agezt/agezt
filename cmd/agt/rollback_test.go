// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRollbackCatalogRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback", "checkpoints.json")
	cp := rollbackCheckpoint{
		ID:           "rb-test",
		Kind:         rollbackCheckpointKindSkill,
		Action:       "apply",
		RunID:        "run-123",
		SubjectID:    "abc123def456",
		SubjectName:  "diagnose-ci",
		BeforeStatus: "draft",
		CreatedMS:    123,
	}
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{cp}}); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	got, err := loadRollbackCatalogAt(path)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if got.Version != rollbackCatalogVersion || len(got.Checkpoints) != 1 {
		t.Fatalf("catalog = %+v, want one v%d checkpoint", got, rollbackCatalogVersion)
	}
	if got.Checkpoints[0].ID != cp.ID || got.Checkpoints[0].BeforeStatus != "draft" || got.Checkpoints[0].RunID != "run-123" {
		t.Fatalf("checkpoint = %+v, want %+v", got.Checkpoints[0], cp)
	}
}

func TestNewSkillStatusRollbackCheckpoint(t *testing.T) {
	now := time.Unix(1700000000, 42)
	cp := newSkillStatusRollbackCheckpoint("reject", "operator rejected", map[string]any{
		"id": "abcdef0123456789", "name": "triage", "status": "shadow", "body": "steps",
	}, now)
	if cp.Kind != rollbackCheckpointKindSkill || cp.Action != "reject" || cp.BeforeStatus != "shadow" {
		t.Fatalf("checkpoint = %+v", cp)
	}
	if !strings.Contains(cp.ID, "abcdef012345") {
		t.Fatalf("checkpoint id %q should include short subject id", cp.ID)
	}
	if cp.Before["body"] != "steps" {
		t.Fatalf("before snapshot missing body: %+v", cp.Before)
	}
}

func TestValidateSkillStatusCheckpointAction(t *testing.T) {
	cases := []struct {
		action string
		status string
		ok     bool
	}{
		{"apply", "draft", true},
		{"apply", "active", false},
		{"reject", "shadow", true},
		{"reject", "active", false},
		{"quarantine", "active", true},
		{"quarantine", "draft", false},
		{"curate.quarantine", "active", true},
		{"curate.quarantine", "shadow", false},
	}
	for _, tc := range cases {
		err := validateSkillStatusCheckpointAction(tc.action, tc.status)
		if tc.ok && err != nil {
			t.Fatalf("%s/%s unexpected error: %v", tc.action, tc.status, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s/%s should error", tc.action, tc.status)
		}
	}
}

func TestCmdRollbackListEmpty(t *testing.T) {
	t.Setenv("AGEZT_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := cmdRollback([]string{"list"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "no checkpoints") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestCmdRollbackListFiltersByRunID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
	cat := rollbackCatalog{Checkpoints: []rollbackCheckpoint{
		{ID: "rb-a", Kind: rollbackCheckpointKindFile, Action: "file.write", RunID: "run-a", SubjectID: "a", CreatedMS: 100},
		{ID: "rb-b", Kind: rollbackCheckpointKindFile, Action: "file.write", RunID: "run-b", SubjectID: "b", CreatedMS: 200},
		{ID: "rb-c", Kind: rollbackCheckpointKindFile, Action: "file.write", RunID: "run-a", SubjectID: "c", CreatedMS: 300},
	}}
	if err := writeRollbackCatalogAt(path, cat); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := cmdRollback([]string{"list", "--run", "run-a", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	var got struct {
		RunID       string               `json:"run_id"`
		Count       int                  `json:"count"`
		Checkpoints []rollbackCheckpoint `json:"checkpoints"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if got.RunID != "run-a" || got.Count != 2 || len(got.Checkpoints) != 2 {
		t.Fatalf("filtered output = %+v", got)
	}
	if got.Checkpoints[0].ID != "rb-c" || got.Checkpoints[1].ID != "rb-a" {
		t.Fatalf("filtered order = %+v", got.Checkpoints)
	}
}

func TestCmdRollbackShowUnknown(t *testing.T) {
	t.Setenv("AGEZT_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := cmdRollback([]string{"show", "rb-missing"}, &out, &errOut); code != 3 {
		t.Fatalf("exit=%d want 3 stderr=%s", code, errOut.String())
	}
}

func TestCmdRollbackApplyRestoresFileSnapshotWithoutDaemon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	target := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(target, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	cp := rollbackCheckpoint{
		ID:          "rb-file",
		Kind:        rollbackCheckpointKindFile,
		Action:      "file.write",
		SubjectID:   target,
		SubjectName: "notes.txt",
		Before: map[string]any{
			"abs_path":    target,
			"exists":      true,
			"content_b64": base64.StdEncoding.EncodeToString([]byte("old")),
			"mode_perm":   int(0o600),
		},
		CreatedMS: 123,
	}
	path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{cp}}); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := cmdRollback([]string{"apply", cp.ID}, &out, &errOut); code != 0 {
		t.Fatalf("rollback apply exit=%d stderr=%s", code, errOut.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("restored content = %q, want old", got)
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if cat.Checkpoints[0].AppliedMS == 0 {
		t.Fatalf("file checkpoint should be marked applied: %+v", cat.Checkpoints[0])
	}
}

func TestCmdRollbackSubcommandHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdRollback([]string{"apply", "--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0 stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "rollback apply") {
		t.Fatalf("help output = %q", out.String())
	}
}

func TestCmdRollbackApplyRestoresSkillStatus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("ok"))})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	c := waitRollbackTestClient(t, dir)

	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: "proposal", Body: "steps"})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	cp := newSkillStatusRollbackCheckpoint("apply", "", map[string]any{
		"id": sk.ID, "name": sk.Name, "status": "draft",
	}, time.Now())
	path := filepath.Join(dir, filepath.FromSlash(rollbackCatalogRelativePath))
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{cp}}); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdSkillPromote, map[string]any{"id": sk.ID}); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got, _, _ := k.Forge().Get(sk.ID); got.Status != skill.StatusShadow {
		t.Fatalf("status before rollback = %s, want shadow", got.Status)
	}

	var out, errOut bytes.Buffer
	if code := cmdRollback([]string{"apply", cp.ID}, &out, &errOut); code != 0 {
		t.Fatalf("rollback apply exit=%d stderr=%s", code, errOut.String())
	}
	got, found, err := k.Forge().Get(sk.ID)
	if err != nil || !found {
		t.Fatalf("get skill after rollback found=%v err=%v", found, err)
	}
	if got.Status != skill.StatusDraft {
		t.Fatalf("status after rollback = %s, want draft", got.Status)
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if len(cat.Checkpoints) != 1 || cat.Checkpoints[0].AppliedMS == 0 {
		t.Fatalf("checkpoint should be marked applied: %+v", cat.Checkpoints)
	}
	hist, err := c.Call(ctx, controlplane.CmdSkillHistory, map[string]any{"id": sk.ID})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	events, _ := hist["events"].([]any)
	foundRestore := false
	for _, raw := range events {
		e, _ := raw.(map[string]any)
		if e["kind"] == "skill.restored" {
			foundRestore = true
		}
	}
	if !foundRestore {
		t.Fatalf("skill.restored not found in history: %+v", events)
	}
}

func TestWorkflowSaveCheckpointAndRollbackRestore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("ok"))})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	c := waitRollbackTestClient(t, dir)

	v1 := filepath.Join(dir, "v1.json")
	v2 := filepath.Join(dir, "v2.json")
	if err := os.WriteFile(v1, []byte(`{"name":"rollback-flow","description":"v1","nodes":[{"id":"start","type":"trigger"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v2, []byte(`{"name":"rollback-flow","description":"v2","nodes":[{"id":"start","type":"trigger"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := cmdWorkflowSave([]string{"--file", v1}, &out, &errOut); code != 0 {
		t.Fatalf("save v1 exit=%d stderr=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cmdWorkflowSave([]string{"--file", v2}, &out, &errOut); code != 0 {
		t.Fatalf("save v2 exit=%d stderr=%s", code, errOut.String())
	}
	path := filepath.Join(dir, filepath.FromSlash(rollbackCatalogRelativePath))
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		t.Fatalf("load rollback catalog: %v", err)
	}
	if len(cat.Checkpoints) != 1 || cat.Checkpoints[0].Kind != rollbackCheckpointKindFlow {
		t.Fatalf("expected one workflow checkpoint, got %+v", cat.Checkpoints)
	}
	res, err := c.Call(ctx, controlplane.CmdWorkflowShow, map[string]any{"ref": "rollback-flow"})
	if err != nil {
		t.Fatalf("show v2: %v", err)
	}
	wf, _ := res["workflow"].(map[string]any)
	if wf["description"] != "v2" {
		t.Fatalf("description before rollback = %v, want v2", wf["description"])
	}

	out.Reset()
	errOut.Reset()
	if code := cmdRollback([]string{"apply", cat.Checkpoints[0].ID}, &out, &errOut); code != 0 {
		t.Fatalf("rollback workflow exit=%d stderr=%s", code, errOut.String())
	}
	res, err = c.Call(ctx, controlplane.CmdWorkflowShow, map[string]any{"ref": "rollback-flow"})
	if err != nil {
		t.Fatalf("show restored: %v", err)
	}
	wf, _ = res["workflow"].(map[string]any)
	if wf["description"] != "v1" {
		t.Fatalf("description after rollback = %v, want v1", wf["description"])
	}
	cat, err = loadRollbackCatalogAt(path)
	if err != nil {
		t.Fatalf("reload rollback catalog: %v", err)
	}
	if cat.Checkpoints[0].AppliedMS == 0 {
		t.Fatalf("workflow checkpoint should be marked applied: %+v", cat.Checkpoints[0])
	}
}

func TestConfigSetCheckpointAndRollbackRestore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	t.Setenv("AGEZT_EMBED_MODEL", "")
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("ok"))})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	c := waitRollbackTestClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdConfigSet([]string{"AGEZT_EMBED_MODEL", "embed-a"}, &out, &errOut); code != 0 {
		t.Fatalf("set embed-a exit=%d stderr=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cmdConfigSet([]string{"AGEZT_EMBED_MODEL", "embed-b"}, &out, &errOut); code != 0 {
		t.Fatalf("set embed-b exit=%d stderr=%s", code, errOut.String())
	}
	if got := configValueForTest(t, c, "AGEZT_EMBED_MODEL"); got != "embed-b" {
		t.Fatalf("config before rollback = %q, want embed-b", got)
	}

	path := filepath.Join(dir, filepath.FromSlash(rollbackCatalogRelativePath))
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		t.Fatalf("load rollback catalog: %v", err)
	}
	var cp rollbackCheckpoint
	found := false
	for i := len(cat.Checkpoints) - 1; i >= 0; i-- {
		if cat.Checkpoints[i].Kind == rollbackCheckpointKindConfig && cat.Checkpoints[i].SubjectID == "AGEZT_EMBED_MODEL" {
			cp = cat.Checkpoints[i]
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("config checkpoint not found: %+v", cat.Checkpoints)
	}
	out.Reset()
	errOut.Reset()
	if code := cmdRollback([]string{"apply", cp.ID}, &out, &errOut); code != 0 {
		t.Fatalf("rollback config exit=%d stderr=%s", code, errOut.String())
	}
	if got := configValueForTest(t, c, "AGEZT_EMBED_MODEL"); got != "embed-a" {
		t.Fatalf("config after rollback = %q, want embed-a", got)
	}
}

func configValueForTest(t *testing.T, c *controlplane.Client, env string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfigValues, nil)
	if err != nil {
		t.Fatalf("config values: %v", err)
	}
	fields, _ := res["fields"].([]any)
	for _, raw := range fields {
		m, _ := raw.(map[string]any)
		if m != nil && m["env"] == env {
			return str(m["value"])
		}
	}
	t.Fatalf("config field %s not found", env)
	return ""
}

func waitRollbackTestClient(t *testing.T, dir string) *controlplane.Client {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := controlplane.NewClient(dir)
		if err == nil {
			return c
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("client could not connect")
	return nil
}
