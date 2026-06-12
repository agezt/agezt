// SPDX-License-Identifier: MIT

package sdk

import (
	"testing"
	"time"
)

func TestParseMails(t *testing.T) {
	raw := []any{
		map[string]any{
			"id": "m-1", "topic": "dm", "from": "planner", "to": "researcher",
			"text": "deploy target?", "ts_unix_ms": float64(1_700_000_000_000),
		},
		map[string]any{
			"id": "m-2", "topic": "help", "from": "worker", "to": "*",
			"text": "stuck", "help": true, "reply_to": "m-1",
		},
		"not-a-map", // tolerated, skipped
	}

	mails := parseMails(raw)
	if len(mails) != 2 {
		t.Fatalf("want 2 mails, got %d", len(mails))
	}

	a := mails[0]
	if a.ID != "m-1" || a.Topic != "dm" || a.From != "planner" || a.To != "researcher" || a.Text != "deploy target?" {
		t.Errorf("mail[0] fields wrong: %+v", a)
	}
	if !a.At.Equal(time.UnixMilli(1_700_000_000_000)) {
		t.Errorf("at = %v", a.At)
	}
	if a.Help || a.ReplyTo != "" {
		t.Errorf("mail[0] flags wrong: %+v", a)
	}

	b := mails[1]
	if !b.Help || b.ReplyTo != "m-1" || b.To != "*" {
		t.Errorf("mail[1] fields wrong: %+v", b)
	}
	// Missing timestamp is the zero time.
	if !b.At.IsZero() {
		t.Errorf("missing ts should be zero time: %v", b.At)
	}
}

func TestMailForName(t *testing.T) {
	cases := []struct {
		name, from, to string
		want           bool
	}{
		{"researcher", "x", "Researcher", true},  // directed, case-insensitive
		{"researcher", "x", "writer", false},     // someone else's DM
		{"researcher", "myapp", "*", true},       // foreign broadcast
		{"researcher", "Researcher", "*", false}, // own broadcast
		{"researcher", "x", "", false},           // plain topic post
		{"", "x", "writer", true},                // firehose matches everything
	}
	for i, c := range cases {
		if got := mailForName(c.name, c.from, c.to); got != c.want {
			t.Errorf("case %d (%q from=%q to=%q): got %v, want %v", i, c.name, c.from, c.to, got, c.want)
		}
	}
}

func TestParseMails_Empty(t *testing.T) {
	if got := parseMails(nil); len(got) != 0 {
		t.Fatalf("nil input: %d", len(got))
	}
	if got := parseMails(map[string]any{}); len(got) != 0 {
		t.Fatalf("non-list input: %d", len(got))
	}
}
