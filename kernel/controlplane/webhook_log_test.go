// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestWebhookLogAndStats(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Journal two deliveries and one failure (as the dispatcher would).
	pub := func(kind event.Kind, url string, payload map[string]any) {
		payload["url"] = url
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "webhook." + string(kind), Kind: kind, Actor: "webhook", Payload: payload,
		}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	pub(event.KindWebhookDelivered, "https://a/hook", map[string]any{"event_kind": "task.received", "status": 200, "attempts": 1})
	pub(event.KindWebhookDelivered, "https://a/hook", map[string]any{"event_kind": "task.completed", "status": 200, "attempts": 1})
	pub(event.KindWebhookFailed, "https://b/hook", map[string]any{"event_kind": "task.failed", "error": "connection refused", "attempts": 3})

	// stats: 3 total, 1 failed, ~33%.
	stats, err := c.Call(context.Background(), controlplane.CmdWebhookStats, nil)
	if err != nil {
		t.Fatalf("webhook stats: %v", err)
	}
	if tot, _ := stats["total"].(float64); int(tot) != 3 {
		t.Errorf("total=%v want 3", stats["total"])
	}
	if f, _ := stats["failed"].(float64); int(f) != 1 {
		t.Errorf("failed=%v want 1", stats["failed"])
	}

	// log --failed: only the failure.
	res, err := c.Call(context.Background(), controlplane.CmdWebhookLog, map[string]any{"failed": true})
	if err != nil {
		t.Fatalf("webhook log: %v", err)
	}
	rows, _ := res["deliveries"].([]any)
	if len(rows) != 1 {
		t.Fatalf("--failed returned %d rows, want 1", len(rows))
	}
	r0, _ := rows[0].(map[string]any)
	if ok, _ := r0["ok"].(bool); ok {
		t.Errorf("failed row marked ok")
	}
	if e, _ := r0["error"].(string); e != "connection refused" {
		t.Errorf("error=%q want 'connection refused'", e)
	}

	// log (all): 3 rows.
	res, _ = c.Call(context.Background(), controlplane.CmdWebhookLog, nil)
	if rows, _ := res["deliveries"].([]any); len(rows) != 3 {
		t.Errorf("log returned %d rows, want 3", len(rows))
	}
}
