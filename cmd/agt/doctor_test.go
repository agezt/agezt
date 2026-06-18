// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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

func TestCheckMemoryStoreFileRepair(t *testing.T) {
	base := t.TempDir()
	memDir := filepath.Join(base, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(memDir, "memory.json")

	if err := os.WriteFile(path, []byte{0xEF, 0xBB, 0xBF, '{', '}', '\n'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if c := checkMemoryStoreFile(base, false); c.Status != statusWarn || !strings.Contains(c.Detail, "BOM") {
		t.Fatalf("BOM should warn before repair, got %+v", c)
	}
	if c := checkMemoryStoreFile(base, true); c.Status != statusOK || !strings.Contains(c.Detail, "BOM removed") {
		t.Fatalf("BOM repair should OK, got %+v", c)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("repair should remove BOM")
	}

	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := checkMemoryStoreFile(base, false); c.Status != statusFail {
		t.Fatalf("corrupt JSON should fail before repair, got %+v", c)
	}
	if c := checkMemoryStoreFile(base, true); c.Status != statusOK || !strings.Contains(c.Detail, "backed up corrupt") {
		t.Fatalf("corrupt repair should backup and recreate, got %+v", c)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "{}" {
		t.Fatalf("repair should recreate empty JSON object, got %q", string(raw))
	}
	matches, err := filepath.Glob(path + ".bad-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one corrupt backup, matches=%v err=%v", matches, err)
	}
}

// TestDoctorSummaryExit verifies the exit-code contract: warnings don't fail by
// default, a FAIL does, and --strict makes warnings fail too (M165).
func TestDoctorSummaryExit(t *testing.T) {
	cases := []struct {
		name   string
		checks []doctorCheck
		strict bool
		want   int
	}{
		{"all ok", []doctorCheck{ok("a", "x"), ok("b", "y")}, false, 0},
		{"a warning, lenient", []doctorCheck{ok("a", "x"), warn("b", "y", "h")}, false, 0},
		{"a failure, lenient", []doctorCheck{ok("a", "x"), fail("b", "y", "h")}, false, 1},
		{"all ok, strict", []doctorCheck{ok("a", "x"), ok("b", "y")}, true, 0},
		{"a warning, strict", []doctorCheck{ok("a", "x"), warn("b", "y", "h")}, true, 1},
		{"a failure, strict", []doctorCheck{ok("a", "x"), fail("b", "y", "h")}, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := renderDoctorText(tc.checks, tc.strict, &out); got != tc.want {
				t.Errorf("exit = %d want %d", got, tc.want)
			}
		})
	}
}

// TestDoctorExitCode is the pure exit-code mapping (M165).
func TestDoctorExitCode(t *testing.T) {
	cases := []struct {
		worst  checkStatus
		strict bool
		want   int
	}{
		{statusOK, false, 0},
		{statusOK, true, 0},
		{statusWarn, false, 0},
		{statusWarn, true, 1},
		{statusFail, false, 1},
		{statusFail, true, 1},
	}
	for _, tc := range cases {
		if got := doctorExitCode(tc.worst, tc.strict); got != tc.want {
			t.Errorf("doctorExitCode(%s, strict=%v) = %d want %d", tc.worst.label(), tc.strict, got, tc.want)
		}
	}
}

func TestDoctorJSONShape(t *testing.T) {
	var out bytes.Buffer
	code := renderDoctorJSON([]doctorCheck{ok("a", "x"), fail("b", "y", "h")}, false, &out)
	if code != 1 {
		t.Errorf("a FAIL → exit 1, got %d", code)
	}
	s := out.String()
	for _, want := range []string{`"healthy"`, `"checks"`, `"status": "FAIL"`, `"hint": "h"`, `"ok": false`} {
		if !strings.Contains(s, want) {
			t.Errorf("json missing %q in:\n%s", want, s)
		}
	}

	// --strict: a WARN-only run reports ok=false and exits 1, but healthy stays
	// true (healthy tracks FAILs only; ok tracks the strict-aware exit verdict).
	var sout bytes.Buffer
	scode := renderDoctorJSON([]doctorCheck{ok("a", "x"), warn("b", "y", "h")}, true, &sout)
	if scode != 1 {
		t.Errorf("strict WARN → exit 1, got %d", scode)
	}
	ss := sout.String()
	for _, want := range []string{`"strict": true`, `"ok": false`, `"healthy": true`} {
		if !strings.Contains(ss, want) {
			t.Errorf("strict json missing %q in:\n%s", want, ss)
		}
	}
}

func TestAgentHealthCheckFromScan(t *testing.T) {
	c := agentHealthCheckFromScan(map[string]any{"degraded_agents": []any{}})
	if c.Status != statusOK {
		t.Fatalf("empty degraded list should be OK, got %+v", c)
	}
	c = agentHealthCheckFromScan(map[string]any{
		"degraded_agents": []any{
			map[string]any{
				"slug": "worker", "failures": float64(3), "threshold": float64(2),
				"doctor_agent": "guardian-health", "self_repair_enabled": true,
			},
		},
	})
	if c.Status != statusWarn {
		t.Fatalf("degraded agent should warn, got %+v", c)
	}
	if !strings.Contains(c.Detail, "worker") || !strings.Contains(c.Hint, "guardian-health") || !strings.Contains(c.Hint, "self-repair") {
		t.Fatalf("agent health warning missing detail/hint: %+v", c)
	}

	c = agentHealthCheckFromScan(map[string]any{
		"degraded_agents": []any{},
		"misconfigured_agents": []any{
			map[string]any{
				"slug": "worker", "issues": []any{"parent_agent: lead is paused"},
				"doctor_agent": "guardian-doctor", "self_repair_enabled": true,
			},
		},
	})
	if c.Status != statusWarn {
		t.Fatalf("misconfigured agent should warn, got %+v", c)
	}
	if !strings.Contains(c.Detail, "worker") || !strings.Contains(c.Detail, "parent_agent: lead is paused") ||
		!strings.Contains(c.Hint, "agent show worker") || !strings.Contains(c.Hint, "guardian-doctor") {
		t.Fatalf("misconfigured warning missing detail/hint: %+v", c)
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

func TestSchedulesCheckFromList(t *testing.T) {
	mk := func(rows ...map[string]any) map[string]any {
		arr := make([]any, len(rows))
		for i, r := range rows {
			arr[i] = r
		}
		return map[string]any{"schedules": arr, "count": float64(len(rows))}
	}

	// No schedules → OK.
	statusWithResident := map[string]any{"schedules": map[string]any{"resident": true}}

	if c := schedulesCheckFromList(map[string]any{"schedules": []any{}}, statusWithResident); c.Status != statusOK {
		t.Errorf("no schedules: status = %v want OK", c.State)
	}

	// An enabled schedule whose last firing failed → WARN, hint names it.
	w := schedulesCheckFromList(mk(
		map[string]any{"id": "morning", "enabled": true, "last_status": "completed", "last_fired_unix_ms": float64(100)},
		map[string]any{"id": "nightly", "enabled": true, "last_status": "failed", "last_fired_unix_ms": float64(200)},
	), statusWithResident)
	if w.Status != statusWarn {
		t.Fatalf("a failed firing: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Hint, "nightly") {
		t.Errorf("hint = %q want to name the failed schedule nightly", w.Hint)
	}

	// "abandoned" counts as a failure too; the most-recent failure wins the hint.
	w2 := schedulesCheckFromList(mk(
		map[string]any{"id": "old", "enabled": true, "last_status": "abandoned", "last_fired_unix_ms": float64(10)},
		map[string]any{"id": "new", "enabled": true, "last_status": "failed", "last_fired_unix_ms": float64(99)},
	), statusWithResident)
	if w2.Status != statusWarn || !strings.Contains(w2.Hint, "new") {
		t.Errorf("abandoned+failed: status=%v hint=%q want WARN naming 'new'", w2.State, w2.Hint)
	}

	// A DISABLED schedule with a past failure is ignored (operator turned it off).
	dis := schedulesCheckFromList(mk(
		map[string]any{"id": "off", "enabled": false, "last_status": "failed", "last_fired_unix_ms": float64(5)},
	), statusWithResident)
	if dis.Status != statusOK {
		t.Errorf("disabled failed schedule: status = %v want OK", dis.State)
	}

	// Enabled, healthy last firing → OK.
	good := schedulesCheckFromList(mk(
		map[string]any{"id": "ok1", "enabled": true, "last_status": "completed", "last_fired_unix_ms": float64(1)},
	), statusWithResident)
	if good.Status != statusOK {
		t.Errorf("healthy: status = %v want OK", good.State)
	}

	noisy := schedulesCheckFromList(mk(
		map[string]any{"id": "guardian-fast", "enabled": true, "last_status": "completed", "last_fired_unix_ms": float64(1), "frequency_warning": "system agent schedule is more frequent than the guardian quiet window"},
	), statusWithResident)
	if noisy.Status != statusWarn {
		t.Fatalf("frequency warning: status=%v want WARN", noisy.State)
	}
	if !strings.Contains(noisy.Detail, "quiet cadence") || !strings.Contains(noisy.Hint, "guardian-fast") {
		t.Errorf("frequency warning detail=%q hint=%q", noisy.Detail, noisy.Hint)
	}

	blocked := schedulesCheckFromList(mk(
		map[string]any{"id": "stale-flow", "enabled": true, "last_status": "completed", "target_status": "blocked", "target_error": "unknown workflow: stale"},
	), statusWithResident)
	if blocked.Status != statusWarn {
		t.Fatalf("blocked target: status=%v want WARN", blocked.State)
	}
	if !strings.Contains(blocked.Detail, "blocked targets") || !strings.Contains(blocked.Hint, "stale-flow") {
		t.Errorf("blocked target detail=%q hint=%q", blocked.Detail, blocked.Hint)
	}

	disabledNoisy := schedulesCheckFromList(mk(
		map[string]any{"id": "off-fast", "enabled": false, "frequency_warning": "agent wake schedule is very frequent"},
	), statusWithResident)
	if disabledNoisy.Status != statusOK {
		t.Errorf("disabled frequency warning: status=%v want OK", disabledNoisy.State)
	}
	if got := scheduleAttentionIDs(mk(
		map[string]any{"id": "guardian-fast", "enabled": true, "frequency_warning": "system agent schedule is more frequent than the guardian quiet window"},
		map[string]any{"id": "stale-flow", "enabled": true, "target_status": "blocked", "target_error": "unknown workflow: stale"},
		map[string]any{"id": "off-fast", "enabled": false, "frequency_warning": "agent wake schedule is very frequent"},
		map[string]any{"id": "off-stale", "enabled": false, "target_status": "blocked", "target_error": "unknown tool: stale"},
		map[string]any{"id": "healthy", "enabled": true},
	)); !reflect.DeepEqual(got, []string{"guardian-fast", "stale-flow"}) {
		t.Fatalf("scheduleAttentionIDs = %v, want [guardian-fast stale-flow]", got)
	}

	residentMissing := schedulesCheckFromList(mk(
		map[string]any{"id": "armed", "enabled": true, "last_status": "completed", "last_fired_unix_ms": float64(1)},
	), map[string]any{"schedules": map[string]any{"resident": false}})
	if residentMissing.Status != statusWarn {
		t.Fatalf("resident missing: status=%v want WARN", residentMissing.State)
	}
	if !strings.Contains(residentMissing.Detail, "cadence resident is not attached") {
		t.Errorf("resident missing detail = %q", residentMissing.Detail)
	}

	// Enabled but never fired (no last_status) → healthy-by-default OK.
	pending := schedulesCheckFromList(mk(
		map[string]any{"id": "fresh", "enabled": true},
	), statusWithResident)
	if pending.Status != statusOK {
		t.Errorf("never-fired: status = %v want OK", pending.State)
	}
}

func TestStandingCheckFromList(t *testing.T) {
	mk := func(rows ...map[string]any) map[string]any {
		arr := make([]any, 0, len(rows))
		for _, r := range rows {
			arr = append(arr, r)
		}
		return map[string]any{"orders": arr}
	}

	none := standingCheckFromList(mk())
	if none.Status != statusOK || !strings.Contains(none.Detail, "no standing wake rules") {
		t.Fatalf("empty standing check = %v %q, want OK/no standing wake rules", none.State, none.Detail)
	}

	good := standingCheckFromList(mk(
		map[string]any{"id": "ops-ready", "enabled": true, "target_status": "ready"},
	))
	if good.Status != statusOK {
		t.Fatalf("ready standing check = %v, want OK", good.State)
	}

	noisy := standingCheckFromList(mk(
		map[string]any{"id": "ops-fast", "enabled": true, "frequency_warning": "event cooldown is below the default 15m guard"},
	))
	if noisy.Status != statusWarn {
		t.Fatalf("frequency warning: status=%v want WARN", noisy.State)
	}
	if !strings.Contains(noisy.Detail, "quiet cadence") || !strings.Contains(noisy.Hint, "ops-fast") {
		t.Errorf("frequency warning detail=%q hint=%q", noisy.Detail, noisy.Hint)
	}

	blocked := standingCheckFromList(mk(
		map[string]any{"id": "ops-dead", "enabled": true, "target_status": "blocked", "target_error": "standing agent ops is retired"},
	))
	if blocked.Status != statusWarn {
		t.Fatalf("blocked target: status=%v want WARN", blocked.State)
	}
	if !strings.Contains(blocked.Detail, "blocked targets") || !strings.Contains(blocked.Hint, "ops-dead") {
		t.Errorf("blocked target detail=%q hint=%q", blocked.Detail, blocked.Hint)
	}

	disabledNoisy := standingCheckFromList(mk(
		map[string]any{"id": "off-fast", "enabled": false, "frequency_warning": "event cooldown is below the default 15m guard"},
	))
	if disabledNoisy.Status != statusOK {
		t.Errorf("disabled frequency warning: status=%v want OK", disabledNoisy.State)
	}
	if got := standingAttentionIDs(mk(
		map[string]any{"id": "ops-fast", "enabled": true, "frequency_warning": "event cooldown is below the default 15m guard"},
		map[string]any{"id": "ops-dead", "enabled": true, "target_status": "blocked", "target_error": "standing agent ops is retired"},
		map[string]any{"id": "off-fast", "enabled": false, "frequency_warning": "event cooldown is below the default 15m guard"},
		map[string]any{"id": "healthy", "enabled": true},
	)); !reflect.DeepEqual(got, []string{"ops-fast", "ops-dead"}) {
		t.Fatalf("standingAttentionIDs = %v, want [ops-fast ops-dead]", got)
	}
}

func TestGuardianNoiseCheckFromList(t *testing.T) {
	mk := func(rows ...map[string]any) map[string]any {
		arr := make([]any, 0, len(rows))
		for _, r := range rows {
			arr = append(arr, r)
		}
		return map[string]any{"profiles": arr}
	}

	none := guardianNoiseCheckFromList(mk())
	if none.Status != statusOK || !strings.Contains(none.Detail, "no active system guardians") {
		t.Fatalf("empty guardian noise = %v %q, want OK/no active guardians", none.State, none.Detail)
	}

	quiet := guardianNoiseCheckFromList(mk(map[string]any{
		"slug":          "guardian-health",
		"system":        true,
		"memory_scope":  "system/guardian-health",
		"max_cost_mc":   float64(50_000_000),
		"max_daily_mc":  float64(50_000_000),
		"trust_ceiling": "L2",
		"tool_deny":     []any{"memory"},
		"noise_policy": map[string]any{
			"silent_on_success":       true,
			"disable_memory_writes":   true,
			"min_notify_severity":     "warning",
			"min_notify_interval_sec": float64(8 * 3600),
		},
	}))
	if quiet.Status != statusOK || !strings.Contains(quiet.Detail, "quiet") {
		t.Fatalf("quiet guardian noise = %v %q, want OK/quiet", quiet.State, quiet.Detail)
	}

	noisy := guardianNoiseCheckFromList(mk(
		map[string]any{
			"slug":          "guardian-fast",
			"system":        true,
			"memory_scope":  "shared",
			"max_cost_mc":   float64(0),
			"max_daily_mc":  float64(200_000_000),
			"trust_ceiling": "L4",
			"noise_policy": map[string]any{
				"min_notify_severity":     "info",
				"min_notify_interval_sec": float64(60),
			},
		},
		map[string]any{"slug": "guardian-old", "system": true, "retired": true},
	))
	if noisy.Status != statusWarn {
		t.Fatalf("noisy guardian noise = %v, want WARN", noisy.State)
	}
	for _, want := range []string{"guardian-fast", "memory writes enabled", "notify below warning", "daily cap missing/high", "trust above L2", "memory scope not isolated"} {
		if !strings.Contains(noisy.Detail, want) {
			t.Fatalf("noisy guardian detail %q missing %q", noisy.Detail, want)
		}
	}
	if !strings.Contains(noisy.Hint, "doctor --repair") {
		t.Fatalf("noisy guardian hint = %q, want doctor --repair", noisy.Hint)
	}
}

func TestGuardianQuietPatch(t *testing.T) {
	got := guardianQuietPatch(map[string]any{
		"slug":          "guardian-fast",
		"trust_ceiling": "L4",
		"tool_allow":    []any{"memory", "notify"},
		"tool_deny":     []any{"notify"},
		"noise_policy": map[string]any{
			"min_notify_severity":     "info",
			"min_notify_interval_sec": float64(60),
		},
	})
	if got["ref"] != "guardian-fast" || got["memory_scope"] != "system/guardian-fast" ||
		got["max_cost_mc"] != doctorGuardianMaxCostMc || got["max_daily_mc"] != doctorGuardianMaxDailyMc ||
		got["trust_ceiling"] != "L2" {
		t.Fatalf("quiet patch core fields wrong: %v", got)
	}
	allow, _ := got["tool_allow"].([]any)
	if !reflect.DeepEqual(allow, []any{"notify"}) {
		t.Fatalf("quiet patch allow = %v, want notify only", allow)
	}
	deny, _ := got["tool_deny"].([]any)
	if !reflect.DeepEqual(deny, []any{"notify", "memory"}) {
		t.Fatalf("quiet patch deny = %v, want notify+memory", deny)
	}
	noise, _ := got["noise_policy"].(map[string]any)
	if noise["silent_on_success"] != true || noise["disable_memory_writes"] != true ||
		noise["min_notify_severity"] != "warning" || noise["min_notify_interval_sec"] != doctorGuardianNotifyCooldownSec {
		t.Fatalf("quiet patch noise wrong: %v", noise)
	}
}

func TestNetguardCheckFromLog(t *testing.T) {
	// No blocks → OK.
	if c := netguardCheckFromLog(map[string]any{"blocks": []any{}, "count": float64(0)}); c.Status != statusOK {
		t.Errorf("no blocks: status = %v want OK", c.State)
	}
	// Missing key → OK (best-effort).
	if c := netguardCheckFromLog(map[string]any{}); c.Status != statusOK {
		t.Errorf("missing blocks: status = %v want OK", c.State)
	}

	// Blocks present → WARN; hint names the most recent target (newest-first).
	w := netguardCheckFromLog(map[string]any{
		"blocks": []any{
			map[string]any{"ts_unix_ms": float64(200), "ip": "169.254.169.254", "tool": "http", "reason": "link-local"},
			map[string]any{"ts_unix_ms": float64(100), "ip": "10.0.0.5", "tool": "browser.read", "reason": "private"},
		},
		"count": float64(2),
	})
	if w.Status != statusWarn {
		t.Fatalf("blocks present: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Detail, "2 egress") {
		t.Errorf("detail = %q want to mention the count", w.Detail)
	}
	if !strings.Contains(w.Hint, "http→169.254.169.254") {
		t.Errorf("hint = %q want to name the most recent target", w.Hint)
	}

	// A block with no tool still names the ip.
	w2 := netguardCheckFromLog(map[string]any{
		"blocks": []any{map[string]any{"ip": "172.16.0.1"}},
		"count":  float64(1),
	})
	if w2.Status != statusWarn || !strings.Contains(w2.Hint, "172.16.0.1") {
		t.Errorf("ip-only: status=%v hint=%q want WARN naming the ip", w2.State, w2.Hint)
	}
}

func TestRateLimitCheckFromStats(t *testing.T) {
	// No throttling → OK.
	if c := rateLimitCheckFromStats(map[string]any{"throttled": float64(0)}); c.Status != statusOK {
		t.Errorf("no throttle: status = %v want OK", c.State)
	}
	// Missing key → OK (best-effort).
	if c := rateLimitCheckFromStats(map[string]any{}); c.Status != statusOK {
		t.Errorf("missing throttled: status = %v want OK", c.State)
	}

	// Throttling with cap/peak → WARN; detail carries the numbers.
	w := rateLimitCheckFromStats(map[string]any{
		"throttled": float64(12), "limit_per_min": float64(60), "worst_used": float64(75),
	})
	if w.Status != statusWarn {
		t.Fatalf("throttled: status = %v want WARN", w.State)
	}
	if !strings.Contains(w.Detail, "12 request") || !strings.Contains(w.Detail, "cap 60/min") || !strings.Contains(w.Detail, "peak 75") {
		t.Errorf("detail = %q want count+cap+peak", w.Detail)
	}

	// Throttling without a recorded cap → WARN, simpler detail.
	w2 := rateLimitCheckFromStats(map[string]any{"throttled": float64(3)})
	if w2.Status != statusWarn || strings.Contains(w2.Detail, "cap ") {
		t.Errorf("no-cap throttle: status=%v detail=%q want WARN without a cap clause", w2.State, w2.Detail)
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

// TestCheckCredentials — the doctor's AWS credential-chain line (M308): always
// informational OK, with a keyless layer (IRSA / SSO / assume-role) called out.
func TestCheckCredentials(t *testing.T) {
	// Keyless IRSA layer → OK, highlighted.
	c := checkCredentials(map[string]any{
		"cred_chain": "AWS chain: vault → env → web_identity=EksRole → default(file+IMDS)",
	})
	if c.Status != statusOK {
		t.Errorf("web identity: status=%v want OK", c.State)
	}
	if !strings.Contains(c.Detail, "keyless: web_identity") {
		t.Errorf("web identity layer should be highlighted; detail=%q", c.Detail)
	}
	// Base chain (no opt-in) → OK, no keyless tag.
	base := checkCredentials(map[string]any{
		"cred_chain": "AWS chain: vault → env → default(file+IMDS)",
	})
	if base.Status != statusOK {
		t.Errorf("base chain: status=%v want OK", base.State)
	}
	if strings.Contains(base.Detail, "keyless") {
		t.Errorf("base chain must not claim a keyless layer; detail=%q", base.Detail)
	}
	// Absent → OK with the default-chain description (never a FAIL).
	none := checkCredentials(map[string]any{})
	if none.Status != statusOK {
		t.Errorf("absent: status=%v want OK", none.State)
	}
}
