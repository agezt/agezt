// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestStatus_ReturnsExpectedShape asserts the wire fields the
// `agt status` CLI relies on. Counts come from the startPair rig:
// one tool ("shell"), zero active runs, freshly-opened kernel
// (uptime < 5s, journal empty).
func TestStatus_ReturnsExpectedShape(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got, _ := res["daemon"].(string); got != brand.Version {
		t.Errorf("daemon = %q want %q", got, brand.Version)
	}
	if got := intOf(res["protocol"]); got != brand.ProtocolVersion {
		t.Errorf("protocol = %d want %d", got, brand.ProtocolVersion)
	}
	if got := intOf(res["tools"]); got != 1 {
		t.Errorf("tools = %d want 1", got)
	}
	if got := intOf(res["active_runs"]); got != 0 {
		t.Errorf("active_runs = %d want 0", got)
	}
	if halted, _ := res["halted"].(bool); halted {
		t.Errorf("halted = true; want false on a freshly-started kernel")
	}
	if got := intOf(res["uptime_seconds"]); got < 0 || got > 5 {
		t.Errorf("uptime_seconds = %d; want 0..5 for a freshly-started kernel", got)
	}
	if got := intOf(res["journal_head"]); got != 0 {
		t.Errorf("journal_head = %d; want 0 on an empty journal", got)
	}
	// Delegation block present and disabled by default (startPair doesn't
	// enable the delegate tool) — M49.
	deleg, ok := res["delegation"].(map[string]any)
	if !ok {
		t.Fatalf("delegation missing or wrong type: %T", res["delegation"])
	}
	if enabled, _ := deleg["enabled"].(bool); enabled {
		t.Errorf("delegation.enabled = true; want false when the delegate tool is off")
	}

	// Autonomy + actionable signals (M130): a fresh kernel has no schedules and
	// no pending approvals, and — with no tenant registry — no tenants field.
	sched, ok := res["schedules"].(map[string]any)
	if !ok {
		t.Fatalf("schedules missing or wrong type: %T", res["schedules"])
	}
	if got := intOf(sched["total"]); got != 0 {
		t.Errorf("schedules.total = %d want 0 on a fresh kernel", got)
	}
	if got := intOf(res["pending_approvals"]); got != 0 {
		t.Errorf("pending_approvals = %d want 0 on a fresh kernel", got)
	}
	if _, present := res["tenants"]; present {
		t.Errorf("tenants field should be absent when multi-tenancy is disabled")
	}
}

// TestStatus_SchedulesAndTenants — status reflects armed scheduled intents (M130)
// and reports a tenant count when a registry is wired.
func TestStatus_SchedulesAndTenants(t *testing.T) {
	k, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	// Arm two schedules, one paused.
	now := time.Now()
	if _, err := k.Schedules().Add("daily report", time.Hour, "", "operator", now); err != nil {
		t.Fatalf("Add schedule: %v", err)
	}
	e2, err := k.Schedules().Add("paused job", time.Hour, "", "operator", now)
	if err != nil {
		t.Fatalf("Add schedule 2: %v", err)
	}
	if _, err := k.Schedules().SetEnabled(e2.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	// Wire a tenant registry with one tenant.
	reg := withTenants(t, srv, dir)
	if _, err := reg.Acquire("acme", now); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	sched, _ := res["schedules"].(map[string]any)
	if got := intOf(sched["total"]); got != 2 {
		t.Errorf("schedules.total = %d want 2", got)
	}
	if got := intOf(sched["enabled"]); got != 1 {
		t.Errorf("schedules.enabled = %d want 1 (one paused)", got)
	}
	if got := intOf(res["tenants"]); got != 1 {
		t.Errorf("tenants = %d want 1", got)
	}
}

// TestStatus_Channels — configured messaging channels are surfaced (M141), with
// inbound/addr/allowlist, and omitted entirely when none are set.
func TestStatus_Channels(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// None configured → no channels key at all.
	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if _, ok := res["channels"]; ok {
		t.Errorf("channels should be absent when none configured, got %v", res["channels"])
	}

	srv.SetChannels([]controlplane.ChannelInfo{
		{Kind: "telegram", Inbound: true, Allowlist: 1},
		{Kind: "discord", Inbound: false, Addr: "127.0.0.1:8850", Allowlist: 2},
	})
	res, err = c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	chans, _ := res["channels"].([]any)
	if len(chans) != 2 {
		t.Fatalf("channels len = %d want 2", len(chans))
	}
	tg, _ := chans[0].(map[string]any)
	if tg["kind"] != "telegram" || tg["inbound"] != true || intOf(tg["allowlist"]) != 1 {
		t.Errorf("telegram entry wrong: %+v", tg)
	}
	dc, _ := chans[1].(map[string]any)
	if dc["kind"] != "discord" || dc["inbound"] != false || dc["addr"] != "127.0.0.1:8850" || intOf(dc["allowlist"]) != 2 {
		t.Errorf("discord entry wrong: %+v", dc)
	}
}

// TestStatus_DelegationCeilings — with the delegate tool on and the M46–M48 caps
// set, status reports the effective ceilings: depth defaults to 1 (unset), and
// the configured fan-out / spend caps are echoed (M49).
func TestStatus_DelegationCeilings(t *testing.T) {
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider:                   mock.New(mock.FinalText("ok")),
		Tools:                      map[string]agent.Tool{"shell": shell.New()},
		SubAgentTool:               true,
		SubAgentMaxDepth:           0, // effective 1
		SubAgentMaxFanout:          3,
		SubAgentMaxSpendMicrocents: 500_000_000, // $0.50
	})

	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	deleg, ok := res["delegation"].(map[string]any)
	if !ok {
		t.Fatalf("delegation missing or wrong type: %T", res["delegation"])
	}
	if enabled, _ := deleg["enabled"].(bool); !enabled {
		t.Errorf("delegation.enabled = false; want true")
	}
	if got := intOf(deleg["max_depth"]); got != 1 {
		t.Errorf("max_depth = %d; want effective default 1", got)
	}
	if got := intOf(deleg["max_fanout"]); got != 3 {
		t.Errorf("max_fanout = %d; want 3", got)
	}
	if got := intOf(deleg["max_spend_microcents"]); got != 500_000_000 {
		t.Errorf("max_spend_microcents = %d; want 500000000", got)
	}
}

// TestStatus_ReflectsHaltAndJournalGrowth verifies the dynamic
// fields (halted, journal_head) actually track state. After
// halting and publishing one event, status must show halted=true
// and journal_head=1.
func TestStatus_ReflectsHaltAndJournalGrowth(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Halt via direct kernel access — exercises the same code path
	// CmdHalt would, without the overhead of a second round-trip.
	k.Halt()

	// Publish one event so journal_head moves off zero. This is the
	// same path real bus publishers take, so it journals normally
	// (not ephemeral).
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "test.status",
		Kind:    event.Kind("test.event"),
		Actor:   "test",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if halted, _ := res["halted"].(bool); !halted {
		t.Error("halted = false; want true after Halt()")
	}
	if got := intOf(res["journal_head"]); got != 1 {
		t.Errorf("journal_head = %d; want 1 after one publish", got)
	}
}
