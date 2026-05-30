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
