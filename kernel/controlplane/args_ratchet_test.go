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
	"roster.go":        3,
	"schedule.go":      1, // residual: enabled bool-or-string switch
	"workflow.go":      3, // residual: enabled/limit/async dual-type switches
	"standing.go":      1, // residual: enabled bool-or-string switch
	"pulse_control.go": 3, // residual: approve/seconds/min_pct dual-type switches
	// server.go/provider_keys.go residuals sit inside auth/validation gates
	// (tenant auth pin, whoami echo, keyEnv/keyTarget funnels) where a wrong
	// type already collapses to an honest refusal — not migration debt.
	"server.go":          2,
	"provider_keys.go":   2,
	"tenant.go":          1, // residual: tenantOf's documented lenient read
	"mcp.go":             1, // residual: enabled bool-or-string switch
	"chatsuggestions.go": 1, // residual: tools string-or-list dual-type switch
	"channels.go":        1, // residual: wgArg's documented lenient read
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
