// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestNetguardLog(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Empty before any block.
	res, err := c.Call(context.Background(), controlplane.CmdNetguardLog, nil)
	if err != nil {
		t.Fatalf("netguard log: %v", err)
	}
	if n, _ := res["count"].(float64); n != 0 {
		t.Fatalf("count=%v on a fresh kernel, want 0", res["count"])
	}

	// Journal a couple of blocked-egress events (as the guard's OnBlock would).
	for _, ip := range []string{"169.254.169.254", "10.0.0.5"} {
		if _, perr := k.Bus().Publish(event.Spec{
			Subject: "netguard.block",
			Kind:    event.KindNetguardBlocked,
			Actor:   "http",
			Payload: map[string]any{"ip": ip, "reason": "blocked", "tool": "http"},
		}); perr != nil {
			t.Fatalf("publish: %v", perr)
		}
	}

	res, err = c.Call(context.Background(), controlplane.CmdNetguardLog, nil)
	if err != nil {
		t.Fatalf("netguard log: %v", err)
	}
	rows, _ := res["blocks"].([]any)
	if len(rows) != 2 {
		t.Fatalf("got %d blocks, want 2", len(rows))
	}
	// Newest-first: the most recent (10.0.0.5) leads.
	first, _ := rows[0].(map[string]any)
	if ip, _ := first["ip"].(string); ip != "10.0.0.5" {
		t.Errorf("first row ip=%q, want 10.0.0.5 (newest-first)", ip)
	}
	if tool, _ := first["tool"].(string); tool != "http" {
		t.Errorf("tool=%q, want http", tool)
	}
}
