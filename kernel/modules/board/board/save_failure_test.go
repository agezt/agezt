// SPDX-License-Identifier: MIT

package board

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSend_FailedSaveDoesNotLandOnTheBoard: a post whose save FAILED must not
// be on the board. The store appends to its in-memory list and then persists;
// when the persist fails, the message stayed in memory — so a post the caller
// saw fail was on the board anyway, and the next successful save wrote it to
// disk. A message the caller was told did not happen had happened.
func TestSend_FailedSaveDoesNotLandOnTheBoard(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i, text := range []string{"first", "second", "third"} {
		if _, err := st.Post("t", "a", text, int64(i)); err != nil {
			t.Fatalf("post %q: %v", text, err)
		}
	}

	// Break the save: replace the board file with a directory that cannot be
	// removed (a non-empty directory) — the atomic write falls back to
	// remove-then-rename, and that remove fails.
	path := filepath.Join(dir, "board.json")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove board.json: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir board.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "occupied"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write the blocking file: %v", err)
	}
	if _, err := st.Post("t", "a", "the-message-that-could-not-be-saved", 99); err == nil {
		t.Fatal("the post should have failed while the board file cannot be written")
	}

	// The failed message must not be on the board.
	for _, m := range st.Read("t", 0) {
		if m.Text == "the-message-that-could-not-be-saved" {
			t.Fatal("a post whose save failed is on the board anyway")
		}
	}

	// Restore the save path and post once more.
	if err := os.Remove(filepath.Join(path, "occupied")); err != nil {
		t.Fatalf("remove the blocking file: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove the blocking directory: %v", err)
	}
	if _, err := st.Post("t", "a", "after-the-failure", 100); err != nil {
		t.Fatalf("post after the failure: %v", err)
	}

	// The board holds the three posts and the one that succeeded — and not the
	// failed one, in memory or on disk.
	msgs := st.Read("t", 0)
	if len(msgs) != 4 {
		t.Errorf("the board holds %d messages, want 4", len(msgs))
	}
	for _, m := range msgs {
		if m.Text == "the-message-that-could-not-be-saved" {
			t.Error("the failed message was written to the board")
		}
	}
	fresh, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	for _, m := range fresh.Read("t", 0) {
		if m.Text == "the-message-that-could-not-be-saved" {
			t.Error("the failed message was written to disk")
		}
	}
}

// TestAck_FailedSaveDoesNotLandOnTheBoard: the same rule for the ack. An ack
// whose write failed must not clear the reader's inbox in memory — the board
// would claim a read that was never persisted, and the next successful save
// would write it.
func TestAck_FailedSaveDoesNotLandOnTheBoard(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	msg, err := st.Send(Message{Topic: "t", From: "a", To: "reader", Text: "read me"}, 1)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	// Break the save: the board file becomes a directory that cannot be removed.
	path := filepath.Join(dir, "board.json")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove board.json: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir board.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "occupied"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write the blocking file: %v", err)
	}
	if _, _, err := st.Ack(msg.ID, "reader"); err == nil {
		t.Fatal("the ack should have failed while the board file cannot be written")
	}

	// The message is still in the reader's inbox: the ack did not happen.
	inbox := st.Inbox("reader", 0, false)
	if len(inbox) != 1 {
		t.Errorf("the reader's inbox holds %d messages, want 1 (the failed ack must not clear it)", len(inbox))
	}
}
