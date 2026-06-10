// SPDX-License-Identifier: MIT

package board

import "testing"

// TestSendInboxReplies_RoundTripAndPersistence: addressed messages get ids,
// land in the recipient's inbox (unanswered first), replies link back and
// clear the inbox, and everything survives a reopen.
func TestSendInboxReplies_RoundTripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	q, err := s.Send(Message{Topic: "dm", From: "planner", To: "researcher", Text: "deploy target?"}, 1000)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if q.ID == "" {
		t.Fatal("Send assigned no id")
	}
	if _, err := s.Send(Message{Topic: "dm", From: "x", To: "Researcher", Text: "second"}, 1001); err != nil {
		t.Fatalf("Send 2: %v", err)
	}

	// Inbox: case-insensitive recipient match, newest first, both waiting.
	in := s.Inbox("RESEARCHER", 0, false)
	if len(in) != 2 || in[0].Text != "second" || in[1].ID != q.ID {
		t.Fatalf("inbox wrong: %+v", in)
	}
	// Plain topic posts never appear in an inbox.
	if _, err := s.Post("dm", "y", "broadcast", 1002); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if got := len(s.Inbox("researcher", 0, false)); got != 2 {
		t.Fatalf("broadcast leaked into inbox: %d", got)
	}

	// Reply links back, clears the unanswered inbox, shows under Replies.
	r, err := s.Send(Message{Topic: "dm", From: "researcher", To: "planner", ReplyTo: q.ID, Text: "prod-eu"}, 1003)
	if err != nil {
		t.Fatalf("reply Send: %v", err)
	}
	if got := s.Inbox("researcher", 0, false); len(got) != 1 || got[0].Text != "second" {
		t.Fatalf("answered message should leave the inbox: %+v", got)
	}
	if got := s.Inbox("researcher", 0, true); len(got) != 2 {
		t.Fatalf("includeAnswered should show both: %+v", got)
	}
	reps := s.Replies(q.ID, 0)
	if len(reps) != 1 || reps[0].ID != r.ID || reps[0].To != "planner" {
		t.Fatalf("replies wrong: %+v", reps)
	}
	if _, ok := s.Get(q.ID); !ok {
		t.Fatal("Get lost the message")
	}

	// Reopen: ids/addressing/links survive.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reps := s2.Replies(q.ID, 0); len(reps) != 1 || reps[0].Text != "prod-eu" {
		t.Fatalf("replies lost on reopen: %+v", reps)
	}
}
