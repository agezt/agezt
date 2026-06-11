// SPDX-License-Identifier: MIT

package board

import "testing"

func TestPostReadTopics(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st.Post("a", "researcher", "first", 100)
	st.Post("b", "", "other", 200)
	st.Post("a", "", "second", 300)

	// Topic filter, newest first.
	got := st.Read("A", 10) // case-insensitive topic match
	if len(got) != 2 || got[0].Text != "second" {
		t.Fatalf("Read(a) = %+v, want [second, first]", got)
	}
	// Unfiltered.
	if len(st.Read("", 10)) != 3 {
		t.Errorf("unfiltered Read = %d, want 3", len(st.Read("", 10)))
	}
	// Limit.
	if len(st.Read("", 1)) != 1 {
		t.Errorf("limited Read should honor limit")
	}
	tp := st.Topics()
	if tp["a"] != 2 || tp["b"] != 1 {
		t.Errorf("Topics = %+v", tp)
	}
}

func TestPersistenceAndFreshOpenSeesWrites(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir)
	if _, err := w.Post("t", "", "durable", 100); err != nil {
		t.Fatal(err)
	}
	// A SECOND, independent Open (mirrors the control plane's read path) sees the
	// committed write — the property the Web UI board view relies on.
	r, _ := Open(dir)
	if got := r.Read("t", 10); len(got) != 1 || got[0].Text != "durable" {
		t.Fatalf("fresh Open did not see committed write: %+v", got)
	}
}

func TestCapAtMaxMessages(t *testing.T) {
	st, _ := Open(t.TempDir())
	for i := 0; i < MaxMessages+50; i++ {
		st.Post("t", "", "m", int64(i))
	}
	if n := len(st.Read("", MaxMessages+100)); n != MaxMessages {
		t.Errorf("board grew to %d, want cap %d", n, MaxMessages)
	}
}

func TestBroadcast_LandsInEveryInboxButSender(t *testing.T) {
	st, _ := Open(t.TempDir())
	if _, err := st.Broadcast("ann", "deploying now", 100); err != nil {
		t.Fatal(err)
	}
	// A peer sees the broadcast in its inbox...
	if got := st.Inbox("worker", 10, false); len(got) != 1 || got[0].Text != "deploying now" {
		t.Fatalf("peer inbox = %+v, want the broadcast", got)
	}
	// ...but the sender does not see its own broadcast.
	if got := st.Inbox("ann", 10, false); len(got) != 0 {
		t.Fatalf("sender saw its own broadcast: %+v", got)
	}
}

func TestHelpRequest_OpenUntilAnswered(t *testing.T) {
	st, _ := Open(t.TempDir())
	h, err := st.HelpRequest("worker", "", "stuck on the build", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !h.Help || h.To != Everyone {
		t.Fatalf("help request shape wrong: %+v", h)
	}
	// Open until answered; it shows in OpenHelp and in a peer's inbox.
	if got := st.OpenHelp(10); len(got) != 1 || got[0].ID != h.ID {
		t.Fatalf("OpenHelp = %+v, want the request", got)
	}
	if got := st.Inbox("helper", 10, false); len(got) != 1 {
		t.Fatalf("helper inbox = %+v, want the open help", got)
	}
	// A reply closes it: gone from OpenHelp and from the unanswered inbox.
	if _, err := st.Send(Message{From: "helper", To: "worker", ReplyTo: h.ID, Text: "try make clean"}, 200); err != nil {
		t.Fatal(err)
	}
	if got := st.OpenHelp(10); len(got) != 0 {
		t.Fatalf("answered help still open: %+v", got)
	}
	if got := st.Inbox("helper", 10, false); len(got) != 0 {
		t.Fatalf("answered help still waiting in inbox: %+v", got)
	}
}

func TestDirectedHelp_OnlyToTarget(t *testing.T) {
	st, _ := Open(t.TempDir())
	st.HelpRequest("worker", "expert", "review my PR", 100)
	if got := st.Inbox("expert", 10, false); len(got) != 1 {
		t.Fatalf("directed help not delivered to target: %+v", got)
	}
	// A third agent should NOT see a directed (non-broadcast) help request.
	if got := st.Inbox("bystander", 10, false); len(got) != 0 {
		t.Fatalf("bystander saw a directed help request: %+v", got)
	}
}
