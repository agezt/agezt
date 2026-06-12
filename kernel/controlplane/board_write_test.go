// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestBoardWrite_SendInboxAckReplies drives the M937 mailbox commands end to
// end over the control plane: an external sender DMs an agent, the inbox shows
// it, a reply threads back, ack clears a broadcast, and the notifier fires for
// every write.
func TestBoardWrite_SendInboxAckReplies(t *testing.T) {
	_, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))

	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	var notified []board.Message
	srv.SetBoard(st, func(m board.Message, _ string) { notified = append(notified, m) })

	ctx := context.Background()

	// DM an agent by name (topic defaults to "dm").
	res, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": "myapp", "to": "researcher", "text": "deploy target?"})
	if err != nil {
		t.Fatalf("board_send: %v", err)
	}
	sent, _ := res["sent"].(map[string]any)
	id, _ := sent["id"].(string)
	if id == "" || sent["topic"] != "dm" || sent["to"] != "researcher" {
		t.Fatalf("sent view wrong: %+v", sent)
	}

	// The recipient's inbox shows it.
	res, err = c.Call(ctx, controlplane.CmdBoardInbox, map[string]any{"to": "researcher"})
	if err != nil {
		t.Fatalf("board_inbox: %v", err)
	}
	if res["count"].(float64) != 1 {
		t.Fatalf("inbox count = %v, want 1", res["count"])
	}

	// Reply threads back to the sender without naming to/topic.
	res, err = c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": "researcher", "reply_to": id, "text": "prod-eu"})
	if err != nil {
		t.Fatalf("reply send: %v", err)
	}
	if rep := res["sent"].(map[string]any); rep["to"] != "myapp" || rep["reply_to"] != id {
		t.Fatalf("reply view wrong: %+v", rep)
	}
	res, err = c.Call(ctx, controlplane.CmdBoardReplies, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("board_replies: %v", err)
	}
	if res["count"].(float64) != 1 {
		t.Fatalf("replies count = %v, want 1", res["count"])
	}
	// Answered → leaves the unanswered inbox.
	res, _ = c.Call(ctx, controlplane.CmdBoardInbox, map[string]any{"to": "researcher"})
	if res["count"].(float64) != 0 {
		t.Fatalf("answered DM should leave the inbox: %v", res["count"])
	}

	// Broadcast lands in every inbox except the sender's; ack clears it for
	// the acker only.
	res, err = c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": "myapp", "to": "*", "text": "heads-up"})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	bcID := res["sent"].(map[string]any)["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdBoardAck, map[string]any{"id": bcID, "by": "researcher"}); err != nil {
		t.Fatalf("board_ack: %v", err)
	}
	res, _ = c.Call(ctx, controlplane.CmdBoardInbox, map[string]any{"to": "researcher"})
	if res["count"].(float64) != 0 {
		t.Fatalf("acked broadcast should leave researcher's inbox: %v", res["count"])
	}
	res, _ = c.Call(ctx, controlplane.CmdBoardInbox, map[string]any{"to": "writer"})
	if res["count"].(float64) != 1 {
		t.Fatalf("broadcast must still wait for writer: %v", res["count"])
	}

	// Topic post is readable via the existing board_read.
	if _, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": "myapp", "topic": "status", "text": "shipped"}); err != nil {
		t.Fatalf("topic post: %v", err)
	}
	res, err = c.Call(ctx, controlplane.CmdBoardRead, map[string]any{"topic": "status"})
	if err != nil {
		t.Fatalf("board_read: %v", err)
	}
	if res["count"].(float64) != 1 {
		t.Fatalf("board_read count = %v, want 1", res["count"])
	}

	// Every write fired the notifier (DM, reply, broadcast, topic post).
	if len(notified) != 4 {
		t.Fatalf("notifier fired %d times, want 4", len(notified))
	}
}

// TestBoardWrite_Validation covers the error paths: writes without the shared
// store, missing text, a post with neither topic nor recipient, a reply to an
// unknown id, and an ack without its required args.
func TestBoardWrite_Validation(t *testing.T) {
	_, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Not wired → writes refuse (reads still work via the fresh-Open fallback).
	if _, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{"topic": "x", "text": "y"}); err == nil ||
		!strings.Contains(err.Error(), "not available") {
		t.Fatalf("unwired send: err = %v, want board-unavailable", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardInbox, map[string]any{"to": "a"}); err != nil {
		t.Fatalf("unwired inbox should fall back to a read-only open: %v", err)
	}

	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	srv.SetBoard(st, nil)

	if _, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{"topic": "x"}); err == nil ||
		!strings.Contains(err.Error(), "text") {
		t.Fatalf("missing text: err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{"text": "x"}); err == nil ||
		!strings.Contains(err.Error(), "topic") {
		t.Fatalf("missing topic and to: err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{"text": "x", "reply_to": "nope"}); err == nil ||
		!strings.Contains(err.Error(), "no message") {
		t.Fatalf("reply to unknown id: err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardAck, map[string]any{"id": "only-id"}); err == nil ||
		!strings.Contains(err.Error(), "by") {
		t.Fatalf("ack without by: err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardAck, map[string]any{"id": "nope", "by": "a"}); err == nil ||
		!strings.Contains(err.Error(), "no message") {
		t.Fatalf("ack unknown id: err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardInbox, map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "to") {
		t.Fatalf("inbox without to: err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdBoardReplies, map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "id") {
		t.Fatalf("replies without id: err = %v", err)
	}
}
