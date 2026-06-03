// SPDX-License-Identifier: MIT

package telegram

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/agezt/agezt/kernel/channel"
)

func utf16Units(s string) int { return len(utf16.Encode([]rune(s))) }

// A message longer than Telegram's 4096-unit limit must go out as several
// sendMessage calls — before M234 the single oversize call was rejected with
// 400 and the answer was lost.
func TestSend_ChunksOverLongMessage(t *testing.T) {
	fb := &fakeBotServer{}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, channel.Allowlist{}, nil)

	long := strings.Repeat("x", 10000) // ~2.4× the limit
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "7", Text: long}); err != nil {
		t.Fatalf("send: %v", err)
	}

	if fb.sentCount() < 3 {
		t.Fatalf("expected >=3 chunks for 10000 chars, got %d", fb.sentCount())
	}
	var rejoined strings.Builder
	for i, m := range fb.sent {
		txt, _ := m["text"].(string)
		if u := utf16Units(txt); u > telegramMaxChars {
			t.Errorf("chunk %d is %d units, over the %d limit", i, u, telegramMaxChars)
		}
		rejoined.WriteString(txt)
	}
	if rejoined.String() != long {
		t.Error("chunks did not rejoin to the original message")
	}
}

// An empty or whitespace-only message is a no-op, not a failed send (M236).
func TestSend_EmptyIsNoOp(t *testing.T) {
	fb := &fakeBotServer{}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, channel.Allowlist{}, nil)

	for _, txt := range []string{"", "   ", "\n\t "} {
		if err := c.Send(context.Background(), channel.Outbound{ChannelID: "7", Text: txt}); err != nil {
			t.Fatalf("empty send should be a no-op (nil), got %v", err)
		}
	}
	if fb.sentCount() != 0 {
		t.Fatalf("empty/whitespace should send nothing, got %d sends", fb.sentCount())
	}
}

// A short message is still a single send (no behaviour change for the common case).
func TestSend_ShortMessageSingleCall(t *testing.T) {
	fb := &fakeBotServer{}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, channel.Allowlist{}, nil)

	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "7", Text: "short answer"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if fb.sentCount() != 1 {
		t.Fatalf("short message should be one send, got %d", fb.sentCount())
	}
}
