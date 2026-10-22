// SPDX-License-Identifier: MIT

package board

import "testing"

// TestAck_ClearsInboxPerReaderAndPersists: an acked message leaves only the
// acker's unanswered inbox (a broadcast stays for the others), acking is
// idempotent, unknown ids report !found, and acks survive a reopen.
func TestAck_ClearsInboxPerReaderAndPersists(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	dm, err := s.Send(Message{Topic: "dm", From: "planner", To: "researcher", Text: "ping"}, 1000)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	bc, err := s.Broadcast("planner", "heads-up", 1001)
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	// Both wait for researcher; the broadcast also waits for a third agent.
	if got := len(s.Inbox("researcher", 0, false)); got != 2 {
		t.Fatalf("inbox before ack: %d", got)
	}

	// Ack the DM: it leaves the unanswered inbox without a reply.
	if _, ok, err := s.Ack(dm.ID, "Researcher"); !ok || err != nil {
		t.Fatalf("Ack dm: ok=%v err=%v", ok, err)
	}
	in := s.Inbox("researcher", 0, false)
	if len(in) != 1 || in[0].ID != bc.ID {
		t.Fatalf("acked DM should leave the inbox: %+v", in)
	}
	// includeAnswered still shows it.
	if got := len(s.Inbox("researcher", 0, true)); got != 2 {
		t.Fatalf("includeAnswered should show both: %d", got)
	}

	// Acking the broadcast hides it for researcher ONLY.
	if _, ok, err := s.Ack(bc.ID, "researcher"); !ok || err != nil {
		t.Fatalf("Ack broadcast: ok=%v err=%v", ok, err)
	}
	if got := len(s.Inbox("researcher", 0, false)); got != 0 {
		t.Fatalf("researcher inbox should be empty: %d", got)
	}
	if got := len(s.Inbox("writer", 0, false)); got != 1 {
		t.Fatalf("broadcast must still wait for writer: %d", got)
	}

	// Idempotent re-ack, blank acker is a no-op, unknown id reports !found.
	if m, ok, err := s.Ack(bc.ID, "researcher"); !ok || err != nil || len(m.AckedBy) != 1 {
		t.Fatalf("re-ack: ok=%v err=%v acked_by=%v", ok, err, m.AckedBy)
	}
	if m, ok, _ := s.Ack(dm.ID, "  "); !ok || len(m.AckedBy) != 1 {
		t.Fatalf("blank acker must not append: %+v", m.AckedBy)
	}
	if _, ok, _ := s.Ack("nope", "researcher"); ok {
		t.Fatal("unknown id should report !found")
	}

	// Reopen: acks survive.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := len(s2.Inbox("researcher", 0, false)); got != 0 {
		t.Fatalf("acks lost on reopen: %d", got)
	}
	if got := len(s2.Inbox("writer", 0, false)); got != 1 {
		t.Fatalf("writer's broadcast lost on reopen: %d", got)
	}
}

func TestBroadcastReplyClearsOnlyReplyingReader(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	bc, err := s.Broadcast("planner", "who can check staging?", 1000)
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if _, err := s.Send(Message{Topic: "broadcast", From: "researcher", To: "planner", ReplyTo: bc.ID, Text: "I can"}, 1001); err != nil {
		t.Fatalf("reply: %v", err)
	}

	if got := s.Inbox("researcher", 0, false); len(got) != 0 {
		t.Fatalf("replying reader should clear its broadcast inbox: %+v", got)
	}
	if got := s.Inbox("writer", 0, false); len(got) != 1 || got[0].ID != bc.ID {
		t.Fatalf("broadcast should still wait for non-replying readers: %+v", got)
	}
	if got := s.Inbox("researcher", 0, true); len(got) != 1 || got[0].ID != bc.ID {
		t.Fatalf("includeAnswered should keep broadcast history for replying reader: %+v", got)
	}
}
