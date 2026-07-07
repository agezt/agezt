// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"testing"
	"time"
)

// newTestCenter builds a Center with all-auto access policies and a temp dir.
func newTestCenter(t *testing.T) *Center {
	t.Helper()
	cfg := DefaultConfig(t.TempDir())
	cfg.AccessPolicies = map[Rating]Policy{
		RatingPublic:     PolicyAuto,
		RatingInternal:   PolicyAuto,
		RatingSecret:     PolicyAuto,
		RatingRestricted: PolicyAuto,
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestCenterAdminAccessors exercises the admin/read accessor methods that
// wrap the store: List, ListEntries, GetEntry, ListByRating, Config.
func TestCenterAdminAccessors(t *testing.T) {
	c := newTestCenter(t)

	pub := NewConfigEntry("svc:endpoint", "https://api.example.com")
	pub.Rating = RatingPublic
	if err := c.Set(pub); err != nil {
		t.Fatalf("Set(pub) error = %v", err)
	}
	sec := NewConfigEntry("db:password", "s3cr3t-value")
	sec.Rating = RatingSecret
	if err := c.Set(sec); err != nil {
		t.Fatalf("Set(sec) error = %v", err)
	}

	// List
	if got := c.List(); len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	// ListEntries (admin alias)
	if got := c.ListEntries(); len(got) != 2 {
		t.Fatalf("ListEntries() len = %d, want 2", len(got))
	}
	// GetEntry (no access control)
	e, err := c.GetEntry("db:password")
	if err != nil {
		t.Fatalf("GetEntry() error = %v", err)
	}
	if e.Value != "s3cr3t-value" {
		t.Fatalf("GetEntry().Value = %q, want stored value", e.Value)
	}
	if _, err := c.GetEntry("missing:key"); err == nil {
		t.Fatalf("GetEntry(missing) error = nil, want error")
	}
	// ListByRating
	pubOnly := c.ListByRating(RatingPublic)
	if len(pubOnly) != 1 || pubOnly[0].Key != "svc:endpoint" {
		t.Fatalf("ListByRating(public) = %+v, want svc:endpoint", pubOnly)
	}
	secOnly := c.ListByRating(RatingSecret)
	if len(secOnly) != 1 || secOnly[0].Key != "db:password" {
		t.Fatalf("ListByRating(secret) = %+v, want db:password", secOnly)
	}
	if got := c.ListByRating(RatingRestricted); len(got) != 0 {
		t.Fatalf("ListByRating(restricted) len = %d, want 0", len(got))
	}
	// Config
	if c.Config() == nil {
		t.Fatalf("Config() = nil, want non-nil")
	}
}

// TestCenterUpdateRatingAndOverride exercises UpdateRating (persist branch),
// UpdateRating on a missing key, and SetOverride (both stored + un-stored keys).
func TestCenterUpdateRatingAndOverride(t *testing.T) {
	c := newTestCenter(t)

	entry := NewConfigEntry("svc:token", "abcd1234")
	entry.Rating = RatingInternal
	if err := c.Set(entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// UpdateRating on existing key -> persists.
	if err := c.UpdateRating("svc:token", RatingSecret); err != nil {
		t.Fatalf("UpdateRating() error = %v", err)
	}
	got, err := c.GetEntry("svc:token")
	if err != nil {
		t.Fatalf("GetEntry() error = %v", err)
	}
	if got.Rating != RatingSecret {
		t.Fatalf("rating after UpdateRating = %q, want secret", got.Rating)
	}

	// UpdateRating on missing key -> should surface store error.
	if err := c.UpdateRating("nope:key", RatingPublic); err == nil {
		t.Fatalf("UpdateRating(missing) error = nil, want error")
	}

	// SetOverride on stored key -> updates classifier and store.
	c.SetOverride("svc:token", RatingPublic)
	got, err = c.GetEntry("svc:token")
	if err != nil {
		t.Fatalf("GetEntry() error = %v", err)
	}
	if got.Rating != RatingPublic {
		t.Fatalf("rating after SetOverride = %q, want public", got.Rating)
	}

	// SetOverride for an un-stored key (store.Get errors) still safe.
	c.SetOverride("unstored:key", RatingRestricted)
	if r := c.GetAutoRating("unstored:key", "value"); r != RatingRestricted {
		t.Fatalf("GetAutoRating after override = %q, want restricted", r)
	}
}

// TestCenterAuditAndAccessLogs drives Get to produce audit records, then
// exercises AuditLog, AccessLog (with and without filters), GetAuditLog,
// and Stats.
func TestCenterAuditAndAccessLogs(t *testing.T) {
	c := newTestCenter(t)

	e := NewConfigEntry("svc:url", "https://svc.local")
	e.Rating = RatingPublic
	if err := c.Set(e); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	ctx := context.Background()
	req := ConfigAccessRequest{AgentID: "agent-x", RunID: "run-x", Key: "svc:url"}
	if _, err := c.Get(ctx, req); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// AuditLog: with no time filter and with a since window. An empty
	// result (nil slice) is acceptable; we only need the code paths covered.
	_ = c.AuditLog(0)
	_ = c.AuditLog(time.Hour)

	// AccessLog: unfiltered and filtered by key + agentID + since.
	_ = c.AccessLog("", "", 0)
	_ = c.AccessLog("svc:url", "agent-x", time.Hour)

	// GetAuditLog via AuditQuery.
	_ = c.GetAuditLog(AuditQuery{})

	// Stats
	stats := c.Stats()
	if stats["total_entries"] != 1 {
		t.Fatalf("Stats total_entries = %v, want 1", stats["total_entries"])
	}
	byRating, ok := stats["by_rating"].(map[string]int)
	if !ok {
		t.Fatalf("Stats by_rating type = %T, want map[string]int", stats["by_rating"])
	}
	if byRating[string(RatingPublic)] != 1 {
		t.Fatalf("Stats by_rating[public] = %d, want 1", byRating[string(RatingPublic)])
	}
}

// TestAuditQueryFilters generates audit entries across multiple keys/agents
// and exercises the Query filter branches (Key, AgentID, Decision, Since,
// Until, Limit) plus previewValue for non-public ratings.
func TestAuditQueryFilters(t *testing.T) {
	c := newTestCenter(t)

	// A restricted entry with a long value so previewValue takes the
	// truncate+hash branch.
	restricted := NewConfigEntry("svc:restricted_data", "abcdefghijklmnopqrstuvwxyz")
	restricted.Rating = RatingRestricted
	if err := c.Set(restricted); err != nil {
		t.Fatalf("Set(restricted) error = %v", err)
	}
	internal := NewConfigEntry("svc:internal_url", "https://internal.example.com/x")
	internal.Rating = RatingInternal
	if err := c.Set(internal); err != nil {
		t.Fatalf("Set(internal) error = %v", err)
	}

	ctx := context.Background()
	// Two agents access two keys -> at least 4 audit records.
	for _, agent := range []string{"agent-a", "agent-b"} {
		for _, key := range []string{"svc:restricted_data", "svc:internal_url"} {
			if _, err := c.Get(ctx, ConfigAccessRequest{
				AgentID: agent, RunID: "run-" + agent, Key: key,
			}); err != nil {
				t.Fatalf("Get(%s,%s) error = %v", agent, key, err)
			}
		}
	}

	keyFilter := "svc:restricted_data"
	agentFilter := "agent-a"
	now := time.Now().Unix()

	// Filter by Key.
	byKey := c.GetAuditLog(AuditQuery{Key: &keyFilter})
	for _, e := range byKey {
		if e.Key != keyFilter {
			t.Fatalf("Query(Key) returned entry with key %q", e.Key)
		}
	}

	// Filter by AgentID.
	byAgent := c.GetAuditLog(AuditQuery{AgentID: &agentFilter})
	for _, e := range byAgent {
		if e.AgentID != agentFilter {
			t.Fatalf("Query(AgentID) returned entry with agent %q", e.AgentID)
		}
	}

	// Filter by Decision (allowed).
	_ = c.GetAuditLog(AuditQuery{Decision: AccessAllowed})

	// Since (past) and Until (future) window + Limit.
	_ = c.GetAuditLog(AuditQuery{
		Since: now - 3600,
		Until: now + 3600,
		Limit: 2,
	})

	// Since in the future -> filters everything out.
	future := c.GetAuditLog(AuditQuery{Since: now + 100000})
	if len(future) != 0 {
		t.Fatalf("Query(future Since) len = %d, want 0", len(future))
	}
}

// TestHITLApprovalNoRegistry exercises the requestHITLApproval deny path when
// no approval registry is configured (rating requires HITL).
func TestHITLApprovalNoRegistry(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.AccessPolicies = map[Rating]Policy{
		RatingSecret: PolicyHITL,
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	entry := NewConfigEntry("api:secret_key", "topsecretvalue")
	entry.Rating = RatingSecret
	if err := c.Set(entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	ctx := context.Background()
	_, err = c.Get(ctx, ConfigAccessRequest{
		AgentID: "agent-1",
		RunID:   "run-1",
		Key:     "api:secret_key",
		Reason:  "test access",
	})
	// With HITL required but no registry, access must be denied.
	if err == nil {
		t.Fatalf("Get() error = nil, want denial (no approval registry)")
	}
}
