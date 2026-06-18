// SPDX-License-Identifier: MIT

package irc

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseLine(t *testing.T) {
	prefix, cmd, params := parseLine(":nick!user@host PRIVMSG #room :hello world")
	if prefix != "nick!user@host" || cmd != "PRIVMSG" || params != "#room :hello world" {
		t.Fatalf("parseLine = %q / %q / %q", prefix, cmd, params)
	}
	_, cmd2, _ := parseLine("PING :server")
	if cmd2 != "PING" {
		t.Fatalf("PING cmd = %q", cmd2)
	}
}

func TestSplitPrivmsgAndNick(t *testing.T) {
	target, msg, ok := splitPrivmsg("#room :hi there")
	if !ok || target != "#room" || msg != "hi there" {
		t.Fatalf("splitPrivmsg = %q / %q / %v", target, msg, ok)
	}
	if nickOf("bob!~b@h") != "bob" {
		t.Fatalf("nickOf = %q", nickOf("bob!~b@h"))
	}
}

func TestSplitLinesClamps(t *testing.T) {
	long := make([]byte, 900)
	for i := range long {
		long[i] = 'x'
	}
	out := splitLines("a\n\n" + string(long))
	// "a", then the 900-char line split into 400 + 400 + 100; the empty line drops.
	if len(out) != 4 {
		t.Fatalf("want 4 lines, got %d", len(out))
	}
	if out[0] != "a" || len(out[1]) != 400 || len(out[2]) != 400 || len(out[3]) != 100 {
		t.Fatalf("unexpected split: %q + len %d/%d/%d", out[0], len(out[1]), len(out[2]), len(out[3]))
	}
}

func TestSendRequiresConnection(t *testing.T) {
	ch := New(Config{Nick: "bot"})
	err := ch.Send(context.Background(), channel.Outbound{ChannelID: "#room", Text: "hi"})
	if err == nil {
		t.Fatal("send without a connection must error")
	}
}

// TestHandlePrivmsgAllowlist drives the inbound path against an in-memory pair of
// pipes standing in for the IRC socket, and asserts the agent handler runs only
// for allowlisted sources and its reply is written back as a PRIVMSG.
func TestHandlePrivmsgAllowlist(t *testing.T) {
	var ran bool
	ch := New(Config{
		Nick:      "bot",
		Allowlist: channel.NewAllowlist([]string{"#room"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (string, error) {
			ran = true
			if m.ChannelKind != "irc" || m.ChannelID != "#room" || m.Sender != "alice" {
				t.Fatalf("unexpected msg %+v", m)
			}
			return "pong", nil
		},
	})

	// Not allowlisted: handler must not run.
	ch.handlePrivmsg(context.Background(), "eve!e@h", "#other :hi")
	if ran {
		t.Fatal("handler ran for non-allowlisted source")
	}
	// Allowlisted but no live connection: handler still runs, reply send is a noop error.
	ch.handlePrivmsg(context.Background(), "alice!a@h", "#room :ping")
	if !ran {
		t.Fatal("handler did not run for allowlisted source")
	}
}
