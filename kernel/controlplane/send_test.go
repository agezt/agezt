// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestSend_RoutesToChannel(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	var gotKind, gotTo, gotText string
	srv.SetChannelSender(func(_ context.Context, kind, id, text string) error {
		gotKind, gotTo, gotText = kind, id, text
		return nil
	})

	res, err := c.Call(context.Background(), controlplane.CmdSend, map[string]any{
		"channel": "Slack", "to": "C1", "text": "deploy finished",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res["sent"] != true {
		t.Errorf("sent = %v want true", res["sent"])
	}
	// Kind is lowercased server-side; to/text pass through.
	if gotKind != "slack" || gotTo != "C1" || gotText != "deploy finished" {
		t.Errorf("sender saw (%q,%q,%q) want (slack,C1,deploy finished)", gotKind, gotTo, gotText)
	}
}

func TestSend_MissingArgs(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	srv.SetChannelSender(func(context.Context, string, string, string) error { return nil })

	// Missing text → error, no send.
	if _, err := c.Call(context.Background(), controlplane.CmdSend, map[string]any{
		"channel": "slack", "to": "C1",
	}); err == nil {
		t.Error("send with no text should error")
	}
}

func TestSend_NoSenderConfigured(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	// No SetChannelSender → "no channels configured".
	if _, err := c.Call(context.Background(), controlplane.CmdSend, map[string]any{
		"channel": "slack", "to": "C1", "text": "hi",
	}); err == nil {
		t.Error("send with no channels configured should error")
	}
}

func TestSend_PropagatesSenderError(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	srv.SetChannelSender(func(context.Context, string, string, string) error {
		return fmt.Errorf("channel %q not configured", "discord")
	})
	if _, err := c.Call(context.Background(), controlplane.CmdSend, map[string]any{
		"channel": "discord", "to": "D1", "text": "hi",
	}); err == nil {
		t.Error("send should surface the sender's error (unknown channel)")
	}
}
