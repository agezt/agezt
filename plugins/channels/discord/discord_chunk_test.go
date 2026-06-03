// SPDX-License-Identifier: MIT

package discord

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

// A message longer than Discord's 2000-char limit must go out as several
// messages — a single oversize POST is rejected, losing the answer (M234).
func TestDiscord_SendChunksOverLongMessage(t *testing.T) {
	_, pub := keypair(t)
	posted := make(chan map[string]any, 16)
	api := discordAPI(t, posted)
	defer api.Close()

	c := New(Config{
		PublicKey: pub, Token: "bot-test", ApplicationID: "APP1",
		BaseURL:    api.URL,
		HTTPClient: api.Client(),
	})

	long := strings.Repeat("y", 5000) // 2.5× the limit
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "C1", Text: long}); err != nil {
		t.Fatalf("send: %v", err)
	}
	close(posted)

	var rejoined strings.Builder
	n := 0
	for m := range posted {
		n++
		txt, _ := m["content"].(string)
		if len([]rune(txt)) > discordMaxChars {
			t.Errorf("chunk %d is %d chars, over the %d limit", n, len([]rune(txt)), discordMaxChars)
		}
		rejoined.WriteString(txt)
	}
	if n < 3 {
		t.Fatalf("expected >=3 chunks for 5000 chars, got %d", n)
	}
	if rejoined.String() != long {
		t.Error("chunks did not rejoin to the original message")
	}
}

// An empty or whitespace-only message is a no-op, not a failed POST (M236).
func TestDiscord_EmptySendIsNoOp(t *testing.T) {
	_, pub := keypair(t)
	posted := make(chan map[string]any, 4)
	api := discordAPI(t, posted)
	defer api.Close()
	c := New(Config{
		PublicKey: pub, Token: "bot-test", ApplicationID: "APP1",
		BaseURL: api.URL, HTTPClient: api.Client(),
	})
	for _, txt := range []string{"", "  ", "\n"} {
		if err := c.Send(context.Background(), channel.Outbound{ChannelID: "C1", Text: txt}); err != nil {
			t.Fatalf("empty send should be a no-op (nil), got %v", err)
		}
	}
	select {
	case m := <-posted:
		t.Fatalf("empty send should post nothing, got %v", m)
	case <-time.After(300 * time.Millisecond):
	}
}

// The slash-command follow-up path (followUp) also delivers agent text and hits
// the 2000-char limit, so it chunks too — a long answer to a /command is posted
// as several follow-up messages, not lost (M234).
func TestDiscord_FollowUpChunksLongAnswer(t *testing.T) {
	priv, pub := keypair(t)
	posted := make(chan map[string]any, 16)
	api := discordAPI(t, posted)
	defer api.Close()

	long := strings.Repeat("z", 5000)
	c := New(Config{
		PublicKey: pub, Token: "bot-test", ApplicationID: "APP1",
		BaseURL:    api.URL,
		HTTPClient: api.Client(),
		Allowlist:  channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, _ channel.UnifiedMessage, _ string) (string, error) {
			return long, nil
		},
	})

	body := []byte(`{"type":2,"id":"I1","token":"tok-xyz","channel_id":"C1","member":{"user":{"id":"U1"}},"data":{"name":"agezt","options":[{"name":"prompt","type":3,"value":"hi"}]}}`)
	if rec := postInteraction(t, c, priv, body, false, ""); rec.Code != 200 {
		t.Fatalf("command ACK code = %d want 200", rec.Code)
	}

	// Drain every follow-up chunk (they arrive promptly once the async handler
	// completes); stop when none arrives within the window.
	var posts []map[string]any
collect:
	for {
		select {
		case m := <-posted:
			posts = append(posts, m)
		case <-time.After(2 * time.Second):
			break collect
		}
	}
	if len(posts) < 3 {
		t.Fatalf("expected >=3 follow-up chunks, got %d", len(posts))
	}
	var rejoined strings.Builder
	for i, m := range posts {
		txt, _ := m["content"].(string)
		if len([]rune(txt)) > discordMaxChars {
			t.Errorf("follow-up chunk %d is %d chars, over %d", i, len([]rune(txt)), discordMaxChars)
		}
		rejoined.WriteString(txt)
	}
	if rejoined.String() != long {
		t.Error("follow-up chunks did not rejoin to the original answer")
	}
}
