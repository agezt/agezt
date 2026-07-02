// SPDX-License-Identifier: MIT

package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestActionTool_PlaywrightSessionTabFixtureE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Playwright browser fixture in short mode")
	}
	driver := ResolveActionDriverPath()
	if strings.TrimSpace(driver) == "" {
		t.Skip("browse.mjs driver not found")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not found: %v", err)
	}
	if err := exec.Command("node", "--input-type=module", "-e", "await import('playwright')").Run(); err != nil {
		t.Skipf("playwright module not installed for node: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		cookieState := "missing"
		if c, err := r.Cookie("agezt_e2e"); err == nil && c.Value == "ok" {
			cookieState = "ok"
		}
		switch r.URL.Path {
		case "/login":
			_, _ = w.Write([]byte(`<!doctype html>
<title>Login</title>
<label>Email <input id="email" name="email"></label>
<button id="continue" onclick="document.cookie='agezt_e2e=ok; path=/'; location.href='/dashboard'">Continue</button>`))
		case "/dashboard":
			_, _ = w.Write([]byte(`<!doctype html>
<title>Dashboard</title>
<main id="welcome">Welcome cookie:` + cookieState + `</main>
<button id="reports" onclick="location.href='/reports'">Reports</button>`))
		case "/reports":
			_, _ = w.Write([]byte(`<!doctype html>
<title>Reports</title>
<main id="report">Report ready cookie:` + cookieState + `</main>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tool := NewAction("node", driver)
	tool.AllowAll = true
	tool.AllowLoopback = true
	tool.SessionRoot = t.TempDir()

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":        srv.URL + "/login",
		"profile":    "session",
		"session_id": "e2e",
		"tab_id":     "main",
		"screenshot": false,
		"actions": []map[string]any{
			{"type": "fill", "selector": "#email", "value": "agent@example.test"},
			{"type": "click", "selector": "#continue"},
			{"type": "wait", "selector": "#welcome"},
		},
		"timeout_ms": 15000,
	}))
	if err != nil {
		t.Fatalf("login Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("login browser action failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Welcome cookie:ok") || !strings.Contains(res.Output, `"tab_id": "main"`) {
		t.Fatalf("login output missing cookie/tab evidence:\n%s", res.Output)
	}
	reportsRef := snapshotRefForSelector(t, res.Output, "#reports")

	click := &ActionVerbTool{Name: ActionVerbClick, Base: tool}
	res, err = click.Invoke(context.Background(), mustRaw(t, map[string]any{
		"session_id":    "e2e",
		"tab_id":        "main",
		"ref":           reportsRef,
		"screenshot":    false,
		"wait_selector": "#report",
		"timeout_ms":    15000,
	}))
	if err != nil {
		t.Fatalf("follow-up Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("follow-up browser action failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Report ready cookie:ok") || !strings.Contains(res.Output, `"url": "`+srv.URL+`/reports"`) {
		t.Fatalf("follow-up output missing report/cookie evidence:\n%s", res.Output)
	}
}

func snapshotRefForSelector(t *testing.T, output, selector string) string {
	t.Helper()
	var out struct {
		Snapshot []struct {
			Ref      string `json:"ref"`
			Selector string `json:"selector"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("parse browser output: %v\n%s", err, output)
	}
	for _, item := range out.Snapshot {
		if item.Selector == selector && item.Ref != "" {
			return item.Ref
		}
	}
	t.Fatalf("snapshot selector %q not found in output:\n%s", selector, output)
	return ""
}
