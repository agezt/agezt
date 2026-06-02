// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/catalog"
)

func TestCmdDoctor_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdDoctor([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "doctor [--json]") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdDoctor_RejectsBadArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdDoctor([]string{"--nope"}, &out, &errOut); code != 2 {
		t.Errorf("unknown arg should be exit 2, got %d", code)
	}
}

// checkBaseDir is the one diagnostic that runs without a daemon — exercise its
// branches directly so the check logic is covered regardless of environment.
func TestCheckBaseDir(t *testing.T) {
	t.Run("writable dir → OK", func(t *testing.T) {
		c := checkBaseDir(t.TempDir(), nil)
		if c.Status != statusOK {
			t.Errorf("writable temp dir should be OK, got %s: %s", c.Status.label(), c.Detail)
		}
	})

	t.Run("resolve error → FAIL", func(t *testing.T) {
		c := checkBaseDir("", errFake("boom"))
		if c.Status != statusFail {
			t.Errorf("resolve error should be FAIL, got %s", c.Status.label())
		}
	})

	t.Run("missing dir → WARN", func(t *testing.T) {
		c := checkBaseDir(t.TempDir()+"/does-not-exist", nil)
		if c.Status != statusWarn {
			t.Errorf("absent dir should be WARN, got %s", c.Status.label())
		}
	})
}

// TestDoctorSummaryExit verifies the exit-code contract: warnings don't fail,
// a FAIL does.
func TestDoctorSummaryExit(t *testing.T) {
	cases := []struct {
		name   string
		checks []doctorCheck
		want   int
	}{
		{"all ok", []doctorCheck{ok("a", "x"), ok("b", "y")}, 0},
		{"a warning", []doctorCheck{ok("a", "x"), warn("b", "y", "h")}, 0},
		{"a failure", []doctorCheck{ok("a", "x"), fail("b", "y", "h")}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := renderDoctorText(tc.checks, &out); got != tc.want {
				t.Errorf("exit = %d want %d", got, tc.want)
			}
		})
	}
}

func TestDoctorJSONShape(t *testing.T) {
	var out bytes.Buffer
	code := renderDoctorJSON([]doctorCheck{ok("a", "x"), fail("b", "y", "h")}, &out)
	if code != 1 {
		t.Errorf("a FAIL → exit 1, got %d", code)
	}
	s := out.String()
	for _, want := range []string{`"healthy"`, `"checks"`, `"status": "FAIL"`, `"hint": "h"`} {
		if !strings.Contains(s, want) {
			t.Errorf("json missing %q in:\n%s", want, s)
		}
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func TestCheckModelReadiness(t *testing.T) {
	cat := catalog.NewEmpty()
	cat.Providers["acme"] = &catalog.Provider{
		ID: "acme", NPM: "@ai-sdk/openai-compatible",
		Models: map[string]*catalog.Model{
			"mini":  {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 32768}},
			"large": {ID: "large", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	}

	// Tool-capable model → OK.
	if c := checkModelReadiness(map[string]any{"model": "large"}, cat); c.Status != statusOK {
		t.Errorf("large should be OK; got %s %q", c.State, c.Detail)
	}
	// Tool-less known model → WARN with the advisory + hint.
	c := checkModelReadiness(map[string]any{"model": "mini"}, cat)
	if c.Status != statusWarn {
		t.Errorf("mini should WARN; got %s", c.State)
	}
	if !strings.Contains(c.Detail, "tool-use") || c.Hint == "" {
		t.Errorf("mini warn should mention tool-use + carry a hint; got %q / %q", c.Detail, c.Hint)
	}
	// Unknown-to-catalog model → OK (no false alarm).
	if c := checkModelReadiness(map[string]any{"model": "ghost"}, cat); c.Status != statusOK {
		t.Errorf("unknown model should be OK; got %s", c.State)
	}
	// Mock / empty model → OK.
	for _, m := range []string{"", "mock"} {
		if c := checkModelReadiness(map[string]any{"model": m}, cat); c.Status != statusOK {
			t.Errorf("model %q should be OK; got %s", m, c.State)
		}
	}
	// Nil catalog → OK (capabilities unknown).
	if c := checkModelReadiness(map[string]any{"model": "mini"}, nil); c.Status != statusOK {
		t.Errorf("nil catalog should be OK; got %s", c.State)
	}
}

// TestSandboxCheckFromStats — the doctor's sandbox verdict: WARN on downgraded
// isolation or limit breaches, OK on full isolation / no executions (M98).
func TestSandboxCheckFromStats(t *testing.T) {
	// Downgraded → WARN.
	w := sandboxCheckFromStats(map[string]any{
		"executions": float64(3), "downgraded": float64(2), "downgrade_rate": 0.667,
	})
	if w.Status != statusWarn {
		t.Errorf("downgraded: status = %v want WARN", w.State)
	}
	// Limit breach (no downgrade) → WARN.
	lb := sandboxCheckFromStats(map[string]any{
		"executions": float64(2), "downgraded": float64(0), "limit_breaches": float64(1),
	})
	if lb.Status != statusWarn {
		t.Errorf("limit breach: status = %v want WARN", lb.State)
	}
	// Full isolation → OK.
	good := sandboxCheckFromStats(map[string]any{
		"executions": float64(5), "downgraded": float64(0),
	})
	if good.Status != statusOK {
		t.Errorf("full isolation: status = %v want OK", good.State)
	}
	// No executions → OK.
	none := sandboxCheckFromStats(map[string]any{"executions": float64(0)})
	if none.Status != statusOK {
		t.Errorf("no execs: status = %v want OK", none.State)
	}
}

func TestProviderCheckFromStats(t *testing.T) {
	// Fallbacks present → WARN, hint names the worst primary.
	w := providerCheckFromStats(map[string]any{
		"routed": float64(10), "fallbacks": float64(3), "fallback_rate": 0.3,
		"fallbacks_by_primary": map[string]any{"openai": float64(2), "anthropic": float64(1)},
	})
	if w.Status != statusWarn {
		t.Fatalf("fallbacks: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Hint, "openai") {
		t.Errorf("hint = %q want to name the worst provider openai", w.Hint)
	}
	// No fallbacks → OK.
	good := providerCheckFromStats(map[string]any{
		"routed": float64(8), "fallbacks": float64(0),
	})
	if good.Status != statusOK {
		t.Errorf("no fallbacks: status = %v want OK", good.State)
	}
	// No routing yet → OK.
	none := providerCheckFromStats(map[string]any{"routed": float64(0)})
	if none.Status != statusOK {
		t.Errorf("no routing: status = %v want OK", none.State)
	}
}

func TestApprovalsCheckFromStats(t *testing.T) {
	// Timeouts present → WARN.
	w := approvalsCheckFromStats(map[string]any{
		"total": float64(5), "timeout": float64(2), "pending": float64(0),
	})
	if w.Status != statusWarn {
		t.Errorf("timeouts: status = %v want WARN", w.State)
	}
	// Some pending, no timeouts → OK (in-flight is normal).
	p := approvalsCheckFromStats(map[string]any{
		"total": float64(3), "timeout": float64(0), "pending": float64(1),
	})
	if p.Status != statusOK {
		t.Errorf("pending: status = %v want OK", p.State)
	}
	// All resolved, none timed out → OK.
	good := approvalsCheckFromStats(map[string]any{
		"total": float64(4), "timeout": float64(0), "pending": float64(0),
	})
	if good.Status != statusOK {
		t.Errorf("resolved: status = %v want OK", good.State)
	}
	// No approvals yet → OK.
	none := approvalsCheckFromStats(map[string]any{"total": float64(0)})
	if none.Status != statusOK {
		t.Errorf("no approvals: status = %v want OK", none.State)
	}
}

func TestTopFailingProvider(t *testing.T) {
	got := topFailingProvider(map[string]any{"a": float64(1), "b": float64(5), "c": float64(2)})
	if got != "b" {
		t.Errorf("topFailingProvider = %q want b", got)
	}
	if topFailingProvider(map[string]any{}) != "" {
		t.Errorf("empty map should yield \"\"")
	}
	if topFailingProvider(nil) != "" {
		t.Errorf("nil should yield \"\"")
	}
}

func TestWebhookCheckFromStats(t *testing.T) {
	// Failures present → WARN, hint names the worst sink.
	w := webhookCheckFromStats(map[string]any{
		"total": float64(10), "delivered": float64(7), "failed": float64(3), "failure_rate": 0.3,
		"by_url": map[string]any{
			"https://a.example/hook": map[string]any{"delivered": float64(5), "failed": float64(1)},
			"https://b.example/hook": map[string]any{"delivered": float64(2), "failed": float64(2)},
		},
	})
	if w.Status != statusWarn {
		t.Fatalf("failures: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Hint, "b.example") {
		t.Errorf("hint = %q want to name the worst sink b.example", w.Hint)
	}
	// All delivered → OK.
	good := webhookCheckFromStats(map[string]any{
		"total": float64(8), "delivered": float64(8), "failed": float64(0),
	})
	if good.Status != statusOK {
		t.Errorf("all delivered: status = %v want OK", good.State)
	}
	// No deliveries yet → OK.
	none := webhookCheckFromStats(map[string]any{"total": float64(0)})
	if none.Status != statusOK {
		t.Errorf("no deliveries: status = %v want OK", none.State)
	}
}

func TestTopFailingWebhook(t *testing.T) {
	got := topFailingWebhook(map[string]any{
		"u1": map[string]any{"delivered": float64(3), "failed": float64(1)},
		"u2": map[string]any{"delivered": float64(0), "failed": float64(4)},
		"u3": map[string]any{"delivered": float64(9), "failed": float64(2)},
	})
	if got != "u2" {
		t.Errorf("topFailingWebhook = %q want u2", got)
	}
	// All-zero failures → "" (no sink to blame).
	if z := topFailingWebhook(map[string]any{
		"u1": map[string]any{"delivered": float64(3), "failed": float64(0)},
	}); z != "" {
		t.Errorf("no failures should yield \"\", got %q", z)
	}
	if topFailingWebhook(nil) != "" {
		t.Errorf("nil should yield \"\"")
	}
}

func TestCheckExposure(t *testing.T) {
	mk := func(servers ...map[string]any) map[string]any {
		arr := make([]any, len(servers))
		for i, s := range servers {
			arr[i] = s
		}
		return map[string]any{"http_servers": arr}
	}

	// A non-loopback server → WARN, naming it.
	w := checkExposure(mk(
		map[string]any{"name": "rest api", "addr": "0.0.0.0:8800", "loopback": false},
		map[string]any{"name": "web ui", "addr": "127.0.0.1:8787", "loopback": true},
	))
	if w.Status != statusWarn {
		t.Errorf("exposed: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Detail, "rest api") || strings.Contains(w.Detail, "web ui") {
		t.Errorf("detail should name only the exposed server: %q", w.Detail)
	}

	// All loopback → OK.
	good := checkExposure(mk(
		map[string]any{"name": "rest api", "addr": "127.0.0.1:8800", "loopback": true},
	))
	if good.Status != statusOK {
		t.Errorf("all-loopback: status = %v want OK", good.State)
	}

	// No HTTP servers → OK.
	if none := checkExposure(map[string]any{}); none.Status != statusOK {
		t.Errorf("no servers: status = %v want OK", none.State)
	}
}

func TestBudgetCheckFromBudget(t *testing.T) {
	// No ceiling → OK.
	if c := budgetCheckFromBudget(map[string]any{"spent_mc": 1.0e9, "ceiling_mc": 0.0}); c.Status != statusOK {
		t.Errorf("no ceiling: status = %v want OK", c.State)
	}
	// Well under → OK. ($5 of $20 = 25%)
	mc := func(usd float64) float64 { return usd * 100 * 10_000_000 }
	if c := budgetCheckFromBudget(map[string]any{"spent_mc": mc(5), "ceiling_mc": mc(20)}); c.Status != statusOK {
		t.Errorf("25%%: status = %v want OK; detail=%q", c.State, c.Detail)
	}
	// Near the ceiling (≥90%) → WARN. ($19 of $20 = 95%)
	near := budgetCheckFromBudget(map[string]any{"spent_mc": mc(19), "ceiling_mc": mc(20)})
	if near.Status != statusWarn {
		t.Errorf("95%%: status = %v want WARN", near.State)
	}
	if !strings.Contains(near.Detail, "near the daily ceiling") {
		t.Errorf("95%% detail = %q want 'near the daily ceiling'", near.Detail)
	}
	// At/over the ceiling → WARN (reached). ($20 of $20)
	reached := budgetCheckFromBudget(map[string]any{"spent_mc": mc(20), "ceiling_mc": mc(20)})
	if reached.Status != statusWarn || !strings.Contains(reached.Detail, "ceiling reached") {
		t.Errorf("100%%: status=%v detail=%q want WARN + 'ceiling reached'", reached.State, reached.Detail)
	}
}

func TestCheckChannels(t *testing.T) {
	mk := func(chans ...map[string]any) map[string]any {
		arr := make([]any, len(chans))
		for i, c := range chans {
			arr[i] = c
		}
		return map[string]any{"channels": arr}
	}

	// A channel with a listen addr but inbound disabled → WARN, naming it.
	w := checkChannels(mk(
		map[string]any{"kind": "slack", "inbound": false, "addr": "127.0.0.1:8840", "allowlist": 1.0},
		map[string]any{"kind": "telegram", "inbound": true, "addr": "", "allowlist": 1.0},
	))
	if w.Status != statusWarn {
		t.Errorf("half-configured: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Detail, "slack") || strings.Contains(w.Detail, "telegram") {
		t.Errorf("detail should name only the half-configured channel: %q", w.Detail)
	}

	// All healthy (inbound, or addr-less outbound-only) → OK.
	good := checkChannels(mk(
		map[string]any{"kind": "discord", "inbound": true, "addr": "127.0.0.1:8850", "allowlist": 2.0},
		map[string]any{"kind": "slack", "inbound": false, "addr": "", "allowlist": 0.0}, // outbound-only by choice
	))
	if good.Status != statusOK {
		t.Errorf("healthy: status = %v want OK; detail=%q", good.State, good.Detail)
	}

	// No channels → OK.
	if none := checkChannels(map[string]any{}); none.Status != statusOK {
		t.Errorf("no channels: status = %v want OK", none.State)
	}
}

func TestDiskCheckFromStats(t *testing.T) {
	// Plenty free → OK.
	good := diskCheckFromStats(map[string]any{
		"journal_bytes": float64(5 << 20), "disk_available": true,
		"disk_free_bytes": float64(50 << 30), "disk_free_pct": 50.0,
	})
	if good.Status != statusOK {
		t.Errorf("50%% free: status = %v want OK", good.State)
	}
	// Low free → WARN.
	low := diskCheckFromStats(map[string]any{
		"journal_bytes": float64(1 << 30), "disk_available": true,
		"disk_free_bytes": float64(5 << 30), "disk_free_pct": 7.0,
	})
	if low.Status != statusWarn {
		t.Errorf("7%% free: status = %v want WARN", low.State)
	}
	// Critically low → FAIL.
	crit := diskCheckFromStats(map[string]any{
		"journal_bytes": float64(1 << 30), "disk_available": true,
		"disk_free_bytes": float64(200 << 20), "disk_free_pct": 1.5,
	})
	if crit.Status != statusFail {
		t.Errorf("1.5%% free: status = %v want FAIL", crit.State)
	}
	// No free-space probe → OK (informational, shows journal size).
	unk := diskCheckFromStats(map[string]any{
		"journal_bytes": float64(3 << 20), "disk_available": false,
	})
	if unk.Status != statusOK {
		t.Errorf("unavailable: status = %v want OK", unk.State)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1536, "1.5 KB"},
		{5 << 20, "5.0 MB"},
		{int64(2.5 * (1 << 30)), "2.5 GB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q want %q", c.n, got, c.want)
		}
	}
}

func TestCatalogCheckFromSync(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	// Fresh sync (2 days ago) → OK.
	fresh := now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)
	if c := catalogCheckFromSync(fresh, now); c.Status != statusOK {
		t.Errorf("fresh: status=%v want OK", c.State)
	}
	// Stale sync (30 days ago) → WARN.
	stale := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	if c := catalogCheckFromSync(stale, now); c.Status != statusWarn {
		t.Errorf("stale: status=%v want WARN", c.State)
	}
	// Never synced (empty) → OK.
	if c := catalogCheckFromSync("", now); c.Status != statusOK {
		t.Errorf("empty: status=%v want OK", c.State)
	}
	// Zero time.Time (pre-sync marshal) → OK, not a bogus huge age.
	if c := catalogCheckFromSync("0001-01-01T00:00:00Z", now); c.Status != statusOK {
		t.Errorf("zero time: status=%v want OK", c.State)
	}
	// Unparseable → OK (never a FAIL).
	if c := catalogCheckFromSync("not-a-time", now); c.Status != statusOK {
		t.Errorf("bad time: status=%v want OK", c.State)
	}
}
