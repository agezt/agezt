// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rawArgCastBaseline is the RATCHET for the typed-args migration (Phase 1.2b):
// the number of raw `req.Args[...].(T)` type assertions each file is allowed
// to carry. args.go's typed accessors (argString/argBool/argInt64/
// argStringList) exist precisely because the inline cast collapses "absent"
// and "wrong type" into the same zero value, turning client typos into silent
// wrong behavior; the accessors were adopted at ~11% and the migration
// stalled. This test stops the count from GROWING: a new handler must use the
// typed accessors, and migrating a file means LOWERING its baseline here (or
// deleting the entry at zero). Do not raise a number to make the test pass —
// that re-opens the silent-failure class the accessors were written to close.
var rawArgCastBaseline = map[string]int{
	// roster.go's 3 residual casts are deliberate polymorphic transports
	// (enabled bool-or-string, older_than_days number-or-string,
	// task_model_chain []any) — not migration debt.
	"roster.go":               3,
	"schedule.go":             1, // residual: enabled bool-or-string switch
	"workflow.go":             3, // residual: enabled/limit/async dual-type switches
	"world.go":                15,
	"standing.go":             15,
	"configcenter_handler.go": 14,
	"pulse_control.go":        10,
	"datalake.go":             10,
	"catalog.go":              9,
	"toolforge.go":            8,
	"steer.go":                8,
	"pulse.go":                8,
	"server.go":               7,
	"provider_keys.go":        7,
	"artifact.go":             7,
	"policy_log.go":           6,
	"edict.go":                6,
	"channel_accounts.go":     6,
	"tool_log.go":             5,
	"tenant.go":               5,
	"schedule_fires.go":       5,
	"runs.go":                 5,
	"planner.go":              5,
	"mcp.go":                  5,
	"journal_grep.go":         5,
	"update_control.go":       4,
	"settings.go":             4,
	"board.go":                4,
	"workboard.go":            3,
	"sandbox.go":              3,
	"webhook_log.go":          2,
	"warden_log.go":           2,
	"tool.go":                 2,
	"seat.go":                 2,
	"provider_log.go":         2,
	"chatsuggestions.go":      2,
	"approvals_log.go":        2,
	"world_log.go":            1,
	"ratelimit_log.go":        1,
	"plan_history.go":         1,
	"okr.go":                  1,
	"memory_log.go":           1,
	"journal_export.go":       1,
	"inbox.go":                1,
	"execution_profiles.go":   1,
	"chatsummary.go":          1,
	"channels.go":             1,
	"cache_stats.go":          1,
	"redact_test_cmd.go":      1,
}

var rawArgCastRe = regexp.MustCompile(`req\.Args\[[^]]+\]\.\(`)

// TestRawArgCasts_Ratchet fails when any non-test file in this package carries
// MORE raw req.Args type assertions than its baseline allows. See the comment
// on rawArgCastBaseline for the rules.
func TestRawArgCasts_Ratchet(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		got := len(rawArgCastRe.FindAll(src, -1))
		allowed := rawArgCastBaseline[name]
		if got > allowed {
			t.Errorf("%s has %d raw req.Args[...].(T) casts (baseline %d) — use the typed accessors in args.go (argString/argBool/argInt64/argStringList) for new code; they distinguish absent from wrong-type instead of silently zeroing typos", name, got, allowed)
		}
		if got < allowed {
			t.Errorf("%s is down to %d raw casts (baseline %d) — lock in the progress: lower its entry in rawArgCastBaseline", name, got, allowed)
		}
	}
}
