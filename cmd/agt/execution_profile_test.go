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
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func TestCmdExecProfileListAndShow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"shell": shell.NewWithWarden(warden.New(nil)),
		},
	})
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
	waitExecProfileClient(t, dir)

	var out, errOut bytes.Buffer
	if code := cmdExecProfile([]string{"list"}, &out, &errOut); code != 0 {
		t.Fatalf("list exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "execution profile(s)") || !strings.Contains(out.String(), "warden") {
		t.Fatalf("list output missing profiles:\n%s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := cmdExecProfile([]string{"show", "warden", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("show exit=%d stderr=%s", code, errOut.String())
	}
	var shown struct {
		Profile map[string]any `json:"profile"`
	}
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil {
		t.Fatalf("decode show: %v\n%s", err, out.String())
	}
	if shown.Profile["id"] != "warden" || shown.Profile["requested_isolation"] != "namespace" {
		t.Fatalf("shown profile = %+v", shown.Profile)
	}

	out.Reset()
	errOut.Reset()
	if code := cmdExecProfile([]string{"check", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("check exit=%d stderr=%s", code, errOut.String())
	}
	var checked struct {
		Count  int              `json:"count"`
		Checks []map[string]any `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &checked); err != nil {
		t.Fatalf("decode check: %v\n%s", err, out.String())
	}
	if checked.Count == 0 || len(checked.Checks) == 0 {
		t.Fatalf("empty check report: %+v", checked)
	}

	out.Reset()
	errOut.Reset()
	if code := cmdExecProfile([]string{"doctor"}, &out, &errOut); code != 0 {
		t.Fatalf("doctor alias exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "execution profile check") {
		t.Fatalf("doctor output missing header:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "selectable run profiles:") {
		t.Fatalf("doctor output missing routable profiles:\n%s", out.String())
	}

	t.Setenv("AGEZT_EXEC_PROFILE_DENY", "warden")
	out.Reset()
	errOut.Reset()
	if code := cmdExecProfile([]string{"doctor"}, &out, &errOut); code != 0 {
		t.Fatalf("policy-filtered doctor exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "selectable run profiles: local") {
		t.Fatalf("doctor output did not filter policy-denied profile:\n%s", out.String())
	}
}

func TestCmdExecProfileRejectsBadArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdExecProfile([]string{"show"}, &out, &errOut); code != 2 {
		t.Fatalf("show without id exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "id required") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cmdExecProfile([]string{"check", "--tenant"}, &out, &errOut); code != 2 {
		t.Fatalf("check missing tenant exit=%d want 2", code)
	}
}

func waitExecProfileClient(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := controlplane.NewClient(dir)
		if err == nil {
			_ = c
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("client could not connect")
}
