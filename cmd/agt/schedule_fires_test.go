// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
)

// TestCmdScheduleFires_HelpExitsCleanly — `--help` prints usage and exits 0
// without needing a daemon (M54).
func TestCmdScheduleFires_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdScheduleFires([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"fires", "outcomes", "runs show"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

func TestScheduleTargetStatusText(t *testing.T) {
	if got := scheduleTargetStatusText(map[string]any{"target_status": "ready"}); got != "ready" {
		t.Fatalf("ready target status = %q, want ready", got)
	}
	blocked := scheduleTargetStatusText(map[string]any{
		"target_status": "blocked",
		"target_error":  "unknown workflow: nightly",
	})
	if blocked != "blocked (unknown workflow: nightly)" {
		t.Fatalf("blocked target status = %q", blocked)
	}
	if got := scheduleTargetStatusText(map[string]any{"target_error": "unknown tool: shell"}); !strings.Contains(got, "unknown tool: shell") {
		t.Fatalf("target error status = %q, want error surfaced", got)
	}
	if got := scheduleTargetStatusText(map[string]any{}); got != "" {
		t.Fatalf("missing target status = %q, want empty", got)
	}
}

// TestCmdScheduleFires_RejectsBadArg — a non-numeric, non-flag positional is a
// usage error (exit 2) before any daemon dial (M54).
func TestCmdScheduleFires_RejectsBadArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdScheduleFires([]string{"notanumber"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject the arg; got %q", errOut.String())
	}
}

// TestCmdScheduleFires_IdFlagNeedsValue — `--id` without a value is a usage
// error before any dial (M55). `--help` documents --id.
func TestCmdScheduleFires_IdFlagNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdScheduleFires([]string{"--id"}, &out, &errOut); code != 2 {
		t.Errorf("--id with no value exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "--id needs a schedule id") {
		t.Errorf("stderr should explain --id needs a value; got %q", errOut.String())
	}
	var h, hErr bytes.Buffer
	cmdScheduleFires([]string{"--help"}, &h, &hErr)
	if !strings.Contains(h.String(), "--id") {
		t.Errorf("--help should document --id; got %q", h.String())
	}
}

// TestCmdSchedule_DispatchesFires — the `fires` subcommand (and its `history`
// alias) reach cmdScheduleFires via the dispatcher (M54).
func TestCmdSchedule_DispatchesFires(t *testing.T) {
	for _, sub := range []string{"fires", "history"} {
		var out, errOut bytes.Buffer
		code := cmdSchedule([]string{sub, "--help"}, &out, &errOut)
		if code != 0 {
			t.Errorf("schedule %s --help exit=%d want 0; stderr=%q", sub, code, errOut.String())
		}
		if !strings.Contains(out.String(), "firings") {
			t.Errorf("schedule %s --help should render fires usage; got %q", sub, out.String())
		}
	}
}

func TestScheduleSystemTaskUsageMatchesCadence(t *testing.T) {
	want := strings.Join(cadence.SystemTasks(), "|")
	if got := scheduleSystemTaskUsage(); got != want {
		t.Fatalf("schedule system task usage = %q want %q", got, want)
	}
}

func TestCmdScheduleAdd_RequiresAgentTaskOrTypedTarget(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdSchedule([]string{"add", "--every", "1h"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "agent task or typed target is required") {
		t.Fatalf("stderr = %q, want typed-target usage", errOut.String())
	}
}

func TestCmdScheduleAdd_ContinuousValidationBeforeDial(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "needs value",
			args: []string{"add", "cycle repo-watch", "--continuous"},
			want: "--continuous needs a cooldown duration",
		},
		{
			name: "rejects extra cadence",
			args: []string{"add", "cycle repo-watch", "--continuous", "5m", "--every", "1h"},
			want: "pass exactly one of --every <dur>, --continuous <dur>, --at <HH:MM>, or --in <dur>",
		},
		{
			name: "rejects schedule-only modifiers",
			args: []string{"add", "cycle repo-watch", "--continuous", "5m", "--days", "mon-fri"},
			want: "--continuous cannot combine with --between, --days, --tz, or --once",
		},
		{
			name: "rejects tiny cooldown",
			args: []string{"add", "cycle repo-watch", "--continuous", "500ms"},
			want: "--continuous cooldown must be at least 1s",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := cmdSchedule(tc.args, &out, &errOut)
			if code != 2 {
				t.Fatalf("exit=%d want 2; stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Fatalf("stderr = %q, want %q", errOut.String(), tc.want)
			}
		})
	}
}

func TestCmdScheduleEdit_ContinuousValidationBeforeDial(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "needs value",
			args: []string{"edit", "sch-1", "--continuous"},
			want: "--continuous needs a value",
		},
		{
			name: "rejects extra cadence",
			args: []string{"edit", "sch-1", "--continuous", "5m", "--in", "10m"},
			want: "pass at most one of --every, --continuous, --at, or --in",
		},
		{
			name: "rejects schedule-only modifiers",
			args: []string{"edit", "sch-1", "--continuous", "5m", "--tz", "Europe/Istanbul"},
			want: "--continuous cannot combine with --between, --days, --tz, or --once",
		},
		{
			name: "rejects tiny cooldown",
			args: []string{"edit", "sch-1", "--continuous", "500ms"},
			want: "bad --continuous duration",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := cmdSchedule(tc.args, &out, &errOut)
			if code != 2 {
				t.Fatalf("exit=%d want 2; stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Fatalf("stderr = %q, want %q", errOut.String(), tc.want)
			}
		})
	}
}

func TestScheduleActionText_RendersStructuredJobTargets(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"workflow", map[string]string{"target": cadence.TargetWorkflow, "workflow": "nightly-sync", "intent": "Nightly label"}, "run workflow nightly-sync"},
		{"system task", map[string]string{"target": cadence.TargetSystemTask, "system_task": cadence.SystemTaskCatalogSync}, "run system task catalog_sync"},
		{"tool", map[string]string{"target": cadence.TargetTool, "tool": "shell"}, "run tool shell"},
		{"agent wake", map[string]string{"agent": "ops", "intent": "check disks"}, "wake ops: check disks"},
		{"plain task", map[string]string{"intent": "check disks"}, "check disks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduleActionText(tc.in); got != tc.want {
				t.Fatalf("scheduleActionText() = %q want %q", got, tc.want)
			}
		})
	}
}

func TestScheduleExecutorText_RendersLLMMode(t *testing.T) {
	cases := []struct {
		name     string
		executor string
		usesLLM  bool
		known    bool
		want     string
	}{
		{"tool no llm", "tool", false, true, "tool/no-llm"},
		{"agent llm", "agent", true, true, "agent/llm"},
		{"unknown mode", "workflow", false, false, "workflow"},
		{"empty", "", true, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduleExecutorText(tc.executor, tc.usesLLM, tc.known); got != tc.want {
				t.Fatalf("scheduleExecutorText() = %q want %q", got, tc.want)
			}
		})
	}
}
