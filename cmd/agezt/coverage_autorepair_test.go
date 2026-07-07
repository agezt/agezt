// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

// TestAutoRepairCooldown covers the default, a valid override, and the invalid
// fallbacks (unparseable + non-positive).
func TestAutoRepairCooldown(t *testing.T) {
	t.Setenv("AGEZT_AUTO_REPAIR_COOLDOWN", "")
	if got := autoRepairCooldown(); got != defaultAutoRepairCooldown {
		t.Errorf("empty override = %v, want default %v", got, defaultAutoRepairCooldown)
	}

	t.Setenv("AGEZT_AUTO_REPAIR_COOLDOWN", "45s")
	if got := autoRepairCooldown(); got != 45*time.Second {
		t.Errorf("override = %v, want 45s", got)
	}

	t.Setenv("AGEZT_AUTO_REPAIR_COOLDOWN", "not-a-duration")
	if got := autoRepairCooldown(); got != defaultAutoRepairCooldown {
		t.Errorf("bad override = %v, want default", got)
	}

	t.Setenv("AGEZT_AUTO_REPAIR_COOLDOWN", "-5s")
	if got := autoRepairCooldown(); got != defaultAutoRepairCooldown {
		t.Errorf("negative override = %v, want default", got)
	}
}

// TestAutoRepairRoutingRollbackProbation mirrors the cooldown coverage for the
// routing-rollback probation window.
func TestAutoRepairRoutingRollbackProbation(t *testing.T) {
	t.Setenv("AGEZT_ROUTING_ROLLBACK_PROBATION", "")
	if got := autoRepairRoutingRollbackProbation(); got != defaultRoutingRollbackProbation {
		t.Errorf("empty = %v, want default", got)
	}
	t.Setenv("AGEZT_ROUTING_ROLLBACK_PROBATION", "2m")
	if got := autoRepairRoutingRollbackProbation(); got != 2*time.Minute {
		t.Errorf("override = %v, want 2m", got)
	}
	t.Setenv("AGEZT_ROUTING_ROLLBACK_PROBATION", "bogus")
	if got := autoRepairRoutingRollbackProbation(); got != defaultRoutingRollbackProbation {
		t.Errorf("bad = %v, want default", got)
	}
}

// TestAutoRepairPayloadString covers nil map, missing key, non-string value,
// and a trimmed string value.
func TestAutoRepairPayloadString(t *testing.T) {
	if got := autoRepairPayloadString(nil, "x"); got != "" {
		t.Errorf("nil map = %q, want empty", got)
	}
	pl := map[string]any{"name": "  hello  ", "num": 42}
	if got := autoRepairPayloadString(pl, "name"); got != "hello" {
		t.Errorf("string value = %q, want hello", got)
	}
	if got := autoRepairPayloadString(pl, "num"); got != "" {
		t.Errorf("non-string value = %q, want empty", got)
	}
	if got := autoRepairPayloadString(pl, "missing"); got != "" {
		t.Errorf("missing key = %q, want empty", got)
	}
}

// TestAutoRepairEscalationTarget covers parent preference, owner fallback, the
// self-reference skip, and the no-hierarchy empty case.
func TestAutoRepairEscalationTarget(t *testing.T) {
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "w", ParentAgent: "lead"}); got != "lead" {
		t.Errorf("parent = %q, want lead", got)
	}
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "w", OwnerAgent: "owner"}); got != "owner" {
		t.Errorf("owner fallback = %q, want owner", got)
	}
	// A parent that is the agent itself must be skipped.
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "w", ParentAgent: "w", OwnerAgent: "owner"}); got != "owner" {
		t.Errorf("self-parent should fall to owner, got %q", got)
	}
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "w"}); got != "" {
		t.Errorf("no hierarchy = %q, want empty", got)
	}
}

// TestAutoRepairEscalationFrom covers the health-policy doctor override and the
// default system:doctor fallback.
func TestAutoRepairEscalationFrom(t *testing.T) {
	if got := autoRepairEscalationFrom(roster.Profile{}); got != "system:doctor" {
		t.Errorf("default = %q, want system:doctor", got)
	}
	p := roster.Profile{HealthPolicy: &roster.HealthPolicy{DoctorAgent: "  doc  "}}
	if got := autoRepairEscalationFrom(p); got != "doc" {
		t.Errorf("doctor override = %q, want doc", got)
	}
	// Blank doctor falls through to the default.
	p2 := roster.Profile{HealthPolicy: &roster.HealthPolicy{DoctorAgent: "  "}}
	if got := autoRepairEscalationFrom(p2); got != "system:doctor" {
		t.Errorf("blank doctor = %q, want system:doctor", got)
	}
}

// TestAutoRepairFingerprint covers the empty-after-trim case and the sorted,
// newline-joined non-empty case.
func TestAutoRepairFingerprint(t *testing.T) {
	if got := autoRepairFingerprint("", "  ", ""); got != "" {
		t.Errorf("all-blank = %q, want empty", got)
	}
	got := autoRepairFingerprint("zeta", "  alpha  ", "", "mid")
	if got != "alpha\nmid\nzeta" {
		t.Errorf("fingerprint = %q, want sorted alpha/mid/zeta", got)
	}
}

// TestAutoRepairReason covers the no-issues base string, a joined issue list,
// and the 700-char truncation guard.
func TestAutoRepairReason(t *testing.T) {
	if got := autoRepairReason(nil); got != "deterministic auto-repair: invalid runtime override(s)" {
		t.Errorf("no issues = %q", got)
	}
	got := autoRepairReason([]string{"a", "b"})
	if !strings.HasSuffix(got, "a; b") {
		t.Errorf("joined = %q, want suffix 'a; b'", got)
	}
	long := strings.Repeat("x", 1000)
	got = autoRepairReason([]string{long})
	if !strings.HasSuffix(got, "...") || len(got) >= 1000 {
		t.Errorf("long reason not truncated: len=%d suffix=%q", len(got), got[len(got)-3:])
	}
}
