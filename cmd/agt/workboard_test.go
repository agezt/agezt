// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestCmdWorkboardRoundTrip(t *testing.T) {
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
	waitWorkboardTestClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdWorkboard([]string{"create", "--title", "Ship workboard", "--assignee", "builder", "--priority", "7", "--idempotency-key", "wb-test", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, errOut.String())
	}
	var created struct {
		Created bool           `json:"created"`
		Task    map[string]any `json:"task"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, out.String())
	}
	id := str(created.Task["id"])
	if id == "" || !created.Created {
		t.Fatalf("created output = %+v", created)
	}

	steps := [][]string{
		{"claim", id, "--agent", "builder", "--run", "run-1"},
		{"heartbeat", id, "--agent", "builder", "--run", "run-1"},
		{"comment", id, "--author", "lead", "--body", "looks good"},
		{"block", id, "--actor", "lead", "--reason", "waiting on token"},
		{"unblock", id, "--actor", "lead"},
		{"complete", id, "--actor", "builder"},
		{"archive", id, "--actor", "lead"},
	}
	for _, args := range steps {
		out.Reset()
		errOut.Reset()
		if code := cmdWorkboard(args, &out, &errOut); code != 0 {
			t.Fatalf("%v exit=%d stderr=%s", args, code, errOut.String())
		}
	}

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"list", "--archived", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("list exit=%d stderr=%s", code, errOut.String())
	}
	var listed struct {
		Count int              `json:"count"`
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v\n%s", err, out.String())
	}
	if listed.Count != 1 || str(listed.Tasks[0]["status"]) != "archived" {
		t.Fatalf("listed = %+v", listed)
	}

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"lanes", "--archived", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("lanes exit=%d stderr=%s", code, errOut.String())
	}
	var lanes struct {
		Count     int              `json:"count"`
		TaskCount int              `json:"task_count"`
		Lanes     []map[string]any `json:"lanes"`
	}
	if err := json.Unmarshal(out.Bytes(), &lanes); err != nil {
		t.Fatalf("decode lanes: %v\n%s", err, out.String())
	}
	if lanes.Count != 1 || lanes.TaskCount != 1 || str(lanes.Lanes[0]["assignee"]) != "builder" {
		t.Fatalf("lanes = %+v", lanes)
	}

	events, err := k.Journal().Tail(20)
	if err != nil {
		t.Fatalf("tail journal: %v", err)
	}
	foundClaim := false
	for _, ev := range events {
		if ev.Kind == event.KindWorkboardTaskClaimed && ev.Subject == "workboard."+id {
			foundClaim = true
		}
	}
	if !foundClaim {
		t.Fatalf("workboard claim event not found in tail")
	}
}

func TestCmdWorkboardRejectsMissingTitle(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdWorkboard([]string{"create"}, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "workboard create") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCmdWorkboardPolicyAndFail(t *testing.T) {
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
	waitWorkboardTestClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdWorkboard([]string{"create", "--title", "Retry policy", "--assignee", "builder", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, errOut.String())
	}
	var created struct {
		Task map[string]any `json:"task"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, out.String())
	}
	id := str(created.Task["id"])

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"policy", id, "--max-attempts", "2", "--escalate-to", "lead", "--actor", "lead"}, &out, &errOut); code != 0 {
		t.Fatalf("policy exit=%d stderr=%s", code, errOut.String())
	}
	task, _ := k.Workboard().Get(id)
	if task.RetryPolicy == nil || task.RetryPolicy.MaxAttempts != 2 || task.RetryPolicy.EscalateTo != "lead" {
		t.Fatalf("policy = %+v", task.RetryPolicy)
	}

	for i, runID := range []string{"run-1", "run-2"} {
		out.Reset()
		errOut.Reset()
		if code := cmdWorkboard([]string{"claim", id, "--agent", "builder", "--run", runID}, &out, &errOut); code != 0 {
			t.Fatalf("claim %d exit=%d stderr=%s", i, code, errOut.String())
		}
		out.Reset()
		errOut.Reset()
		if code := cmdWorkboard([]string{"fail", id, "--actor", "builder", "--reason", "provider timeout", "--json"}, &out, &errOut); code != 0 {
			t.Fatalf("fail %d exit=%d stderr=%s", i, code, errOut.String())
		}
	}
	task, _ = k.Workboard().Get(id)
	if task.Status != "blocked" || !strings.Contains(task.BlockReason, "escalate to lead") || len(task.Attempts) != 2 {
		t.Fatalf("task after fails = %+v", task)
	}
}

func TestCmdWorkboardDispatchAndWatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("ready for review"))})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Soul: "You build AGEZT tasks."}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	waitWorkboardTestClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdWorkboard([]string{"create", "--title", "Dispatch me", "--assignee", "builder", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, errOut.String())
	}
	var created struct {
		Task map[string]any `json:"task"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, out.String())
	}
	id := str(created.Task["id"])

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"create", "--title", "Prerequisite", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("create prereq exit=%d stderr=%s", code, errOut.String())
	}
	var prereq struct {
		Task map[string]any `json:"task"`
	}
	if err := json.Unmarshal(out.Bytes(), &prereq); err != nil {
		t.Fatalf("decode prereq: %v\n%s", err, out.String())
	}
	prereqID := str(prereq.Task["id"])
	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"depend", id, "--on", prereqID, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("depend exit=%d stderr=%s", code, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"dispatch", id, "--json"}, &out, &errOut); code == 0 {
		t.Fatalf("dispatch with blocked dependency unexpectedly succeeded: stdout=%s", out.String())
	}
	if !strings.Contains(errOut.String(), "blocked by dependencies") {
		t.Fatalf("dispatch blocked stderr=%q", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"complete", prereqID, "--actor", "lead"}, &out, &errOut); code != 0 {
		t.Fatalf("complete prereq exit=%d stderr=%s", code, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"dispatch", id, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("dispatch exit=%d stderr=%s", code, errOut.String())
	}
	var dispatched struct {
		Accepted      bool   `json:"accepted"`
		Agent         string `json:"agent"`
		CorrelationID string `json:"correlation_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &dispatched); err != nil {
		t.Fatalf("decode dispatch: %v\n%s", err, out.String())
	}
	if !dispatched.Accepted || dispatched.Agent != "builder" || dispatched.CorrelationID == "" {
		t.Fatalf("dispatch output = %+v", dispatched)
	}

	waitWorkboardStatus(t, k, id, "review")

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"watch", id, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("watch exit=%d stderr=%s", code, errOut.String())
	}
	var watched struct {
		RunID  string           `json:"run_id"`
		Task   map[string]any   `json:"task"`
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(out.Bytes(), &watched); err != nil {
		t.Fatalf("decode watch: %v\n%s", err, out.String())
	}
	if watched.RunID != dispatched.CorrelationID {
		t.Fatalf("watch run_id=%q want %q", watched.RunID, dispatched.CorrelationID)
	}
	if str(watched.Task["status"]) != "review" {
		t.Fatalf("watch task status=%q", str(watched.Task["status"]))
	}
	foundDispatch := false
	for _, ev := range watched.Events {
		if str(ev["kind"]) == string(event.KindWorkboardTaskDispatched) && str(ev["correlation_id"]) == dispatched.CorrelationID {
			foundDispatch = true
		}
	}
	if !foundDispatch {
		t.Fatalf("dispatch event missing from watch events: %+v", watched.Events)
	}
}

func TestCmdWorkboardDispatchRetriesAndEscalates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	prov := mock.New()
	calls := 0
	prov.OnRequest = func(agent.CompletionRequest) { calls++ }
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: prov})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Soul: "You build AGEZT tasks."}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	waitWorkboardTestClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdWorkboard([]string{"create", "--title", "Retry dispatch", "--assignee", "builder", "--max-attempts", "2", "--escalate-to", "lead", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, errOut.String())
	}
	var created struct {
		Task map[string]any `json:"task"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, out.String())
	}
	id := str(created.Task["id"])

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"dispatch", id, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("dispatch exit=%d stderr=%s", code, errOut.String())
	}
	waitWorkboardStatus(t, k, id, "blocked")
	task, _ := k.Workboard().Get(id)
	if len(task.Attempts) != 2 || task.Attempts[0].Status != "failed" || task.Attempts[1].Status != "failed" {
		t.Fatalf("attempts = %+v", task.Attempts)
	}
	if !strings.Contains(task.BlockReason, "escalate to lead") {
		t.Fatalf("block_reason = %q", task.BlockReason)
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
}

func TestCmdWorkboardSweepReclaimsStaleClaims(t *testing.T) {
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
	waitWorkboardTestClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdWorkboard([]string{"create", "--title", "Sweep me", "--assignee", "builder", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, errOut.String())
	}
	var created struct {
		Task map[string]any `json:"task"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, out.String())
	}
	id := str(created.Task["id"])

	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"claim", id, "--agent", "builder", "--run", "run-sweep"}, &out, &errOut); code != 0 {
		t.Fatalf("claim exit=%d stderr=%s", code, errOut.String())
	}
	time.Sleep(5 * time.Millisecond)
	out.Reset()
	errOut.Reset()
	if code := cmdWorkboard([]string{"sweep", "--stale-after", "1ms", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("sweep exit=%d stderr=%s", code, errOut.String())
	}
	var swept struct {
		ReclaimedCount int              `json:"reclaimed_count"`
		Tasks          []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(out.Bytes(), &swept); err != nil {
		t.Fatalf("decode sweep: %v\n%s", err, out.String())
	}
	if swept.ReclaimedCount != 1 || len(swept.Tasks) != 1 || str(swept.Tasks[0]["status"]) != "ready" {
		t.Fatalf("swept = %+v", swept)
	}
	task, _ := k.Workboard().Get(id)
	if task.Claim != nil || task.Status != "ready" {
		t.Fatalf("task after sweep = %+v", task)
	}
}

func waitWorkboardTestClient(t *testing.T, dir string) *controlplane.Client {
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

func waitWorkboardStatus(t *testing.T, k *runtime.Kernel, id, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, ok := k.Workboard().Get(id)
		if ok && string(task.Status) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := k.Workboard().Get(id)
	t.Fatalf("task %s status=%q want %q", id, task.Status, want)
}
