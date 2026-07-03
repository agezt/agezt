// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestInboxEmpty(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdInbox, nil)
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	threads, ok := res["threads"].([]any)
	if !ok || len(threads) != 0 {
		t.Fatalf("empty inbox should return [], got %v", res["threads"])
	}
}

func TestInboxGroupsByCorrelation(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Simulate a Telegram exchange: one inbound + its reply, sharing corr.
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "channel.inbound.telegram", Kind: event.KindChannelInbound,
		Actor: "channel-telegram", CorrelationID: "chan-1",
		Payload: map[string]any{"channel_kind": "telegram", "channel_id": "42", "sender": "ersin", "text": "hi"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "channel.outbound.telegram", Kind: event.KindChannelOutbound,
		Actor: "channel-telegram", CorrelationID: "chan-1",
		Payload: map[string]any{"channel_kind": "telegram", "channel_id": "42", "text": "hello back"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdInbox, nil)
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	threads, _ := res["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("inbound+outbound with same corr should be ONE thread, got %d", len(threads))
	}
	th, _ := threads[0].(map[string]any)
	if th["channel_id"] != "42" {
		t.Errorf("thread channel_id = %v", th["channel_id"])
	}
	msgs, _ := th["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("thread should have 2 messages, got %d", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["direction"] != "in" || first["text"] != "hi" {
		t.Errorf("first message wrong: %+v", first)
	}
}

func TestInboxFilterByChannel(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// One thread per channel kind, each a single inbound.
	for kind, id := range map[string]string{"telegram": "42", "slack": "C1", "discord": "D9"} {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "channel.inbound." + kind, Kind: event.KindChannelInbound,
			Actor: "channel-" + kind, CorrelationID: "corr-" + kind,
			Payload: map[string]any{"channel_kind": kind, "channel_id": id, "text": "hi from " + kind},
		})
	}

	// Unfiltered: all three.
	res, err := c.Call(context.Background(), controlplane.CmdInbox, nil)
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if threads, _ := res["threads"].([]any); len(threads) != 3 {
		t.Fatalf("unfiltered should show 3 threads, got %d", len(threads))
	}

	// Filtered to discord (case-insensitive): exactly one, the discord thread.
	res, err = c.Call(context.Background(), controlplane.CmdInbox, map[string]any{"channel": "DISCORD"})
	if err != nil {
		t.Fatalf("inbox(channel=discord): %v", err)
	}
	if res["channel"] != "discord" {
		t.Errorf("echoed channel = %v want discord", res["channel"])
	}
	threads, _ := res["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("discord filter should show 1 thread, got %d", len(threads))
	}
	th, _ := threads[0].(map[string]any)
	if th["channel_kind"] != "discord" || th["channel_id"] != "D9" {
		t.Errorf("wrong thread survived filter: %+v", th)
	}

	// A channel with no messages → empty.
	res, _ = c.Call(context.Background(), controlplane.CmdInbox, map[string]any{"channel": "matrix"})
	if threads, _ := res["threads"].([]any); len(threads) != 0 {
		t.Errorf("unknown channel filter should be empty, got %d", len(threads))
	}
}

func TestInboxNewestFirst(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	for _, id := range []string{"a", "b"} {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "channel.inbound.telegram", Kind: event.KindChannelInbound,
			Actor: "channel-telegram", CorrelationID: "corr-" + id,
			Payload: map[string]any{"channel_id": id, "text": id},
		})
	}
	res, _ := c.Call(context.Background(), controlplane.CmdInbox, nil)
	threads, _ := res["threads"].([]any)
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	// "b" published last → newest → first.
	top, _ := threads[0].(map[string]any)
	if top["channel_id"] != "b" {
		t.Errorf("newest thread should be first; got %v", top["channel_id"])
	}
}

func TestInbox_CursorPaginatesByLastTSUnixMSDesc(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	// Five independent threads with distinct correlations.
	corrs := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	for _, corr := range corrs {
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "channel.inbound.telegram", Kind: event.KindChannelInbound,
			Actor: "channel-telegram", CorrelationID: corr,
			Payload: map[string]any{"channel_kind": "telegram", "channel_id": corr, "sender": "u", "text": "hi " + corr},
		}); err != nil {
			t.Fatalf("publish %s: %v", corr, err)
		}
	}

	p1, err := c.Call(ctx, controlplane.CmdInbox, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("inbox p1: %v", err)
	}
	threads1, _ := p1["threads"].([]any)
	if len(threads1) != 2 {
		t.Fatalf("page 1 should have 2 threads, got %d", len(threads1))
	}
	if p1["next_cursor"] == "" || p1["next_cursor"] == nil {
		t.Fatal("page 1 should have next_cursor")
	}
	if intOf(p1["total"]) != 5 {
		t.Fatalf("page 1 total wrong: %v", p1["total"])
	}

	p2, err := c.Call(ctx, controlplane.CmdInbox, map[string]any{
		"limit": 2, "cursor": p1["next_cursor"],
	})
	if err != nil {
		t.Fatalf("inbox p2: %v", err)
	}
	threads2, _ := p2["threads"].([]any)
	if len(threads2) != 2 {
		t.Fatalf("page 2 should have 2 threads, got %d", len(threads2))
	}
	// No duplicate correlation IDs across pages.
	seen := map[string]bool{}
	for _, raw := range threads1 {
		if th, _ := raw.(map[string]any); th != nil {
			seen[th["correlation_id"].(string)] = true
		}
	}
	for _, raw := range threads2 {
		if th, _ := raw.(map[string]any); th != nil {
			if seen[th["correlation_id"].(string)] {
				t.Fatalf("corr %s appeared on both pages", th["correlation_id"])
			}
		}
	}

	p3, err := c.Call(ctx, controlplane.CmdInbox, map[string]any{
		"limit": 2, "cursor": p2["next_cursor"],
	})
	if err != nil {
		t.Fatalf("inbox p3: %v", err)
	}
	threads3, _ := p3["threads"].([]any)
	if len(threads3) != 1 {
		t.Fatalf("page 3 should have 1 thread, got %d", len(threads3))
	}
	if _, has := p3["next_cursor"]; has {
		t.Fatalf("page 3 should not have next_cursor, got %v", p3["next_cursor"])
	}
}

func TestInbox_UnparseableCursorFallsBackToFirstPage(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	for _, corr := range []string{"alpha", "bravo"} {
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "channel.inbound.telegram", Kind: event.KindChannelInbound,
			Actor: "channel-telegram", CorrelationID: corr,
			Payload: map[string]any{"channel_kind": "telegram", "channel_id": corr, "sender": "u", "text": "hi " + corr},
		}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	res, err := c.Call(ctx, controlplane.CmdInbox, map[string]any{
		"limit": 10, "cursor": "garbage",
	})
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	threads, _ := res["threads"].([]any)
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
}
