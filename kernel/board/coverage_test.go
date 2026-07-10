// SPDX-License-Identifier: MIT

package board

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpen_MkdirError forces os.MkdirAll to fail by pointing the board dir at a
// path whose parent is an existing regular file (can't create a child dir).
func TestOpen_MkdirError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// afile is a file, so afile/sub can't be created as a directory.
	if _, err := Open(filepath.Join(file, "sub")); err == nil {
		t.Fatalf("Open: expected mkdir error")
	}
}

// TestOpen_ReadError makes board.json a directory so os.ReadFile returns a
// non-IsNotExist error, exercising the read-error branch.
func TestOpen_ReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "board.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatalf("Open: expected read error when board.json is a directory")
	}
}

// TestGet_NotFound exercises the miss return path.
func TestGet_NotFound(t *testing.T) {
	st, _ := Open(t.TempDir())
	if _, ok := st.Get("nope"); ok {
		t.Fatalf("Get(nope) should not be found")
	}
	m, _ := st.Post("t", "a", "hi", 1)
	if got, ok := st.Get(m.ID); !ok || got.ID != m.ID {
		t.Fatalf("Get(existing) failed: %+v ok=%v", got, ok)
	}
}

// TestOpenHelp_LimitTruncation exercises the limit>0 && len>limit branch.
func TestOpenHelp_LimitTruncation(t *testing.T) {
	st, _ := Open(t.TempDir())
	st.HelpRequest("a", "", "need1", 1)
	st.HelpRequest("b", "", "need2", 2)
	st.HelpRequest("c", "", "need3", 3)
	got := st.OpenHelp(2)
	if len(got) != 2 {
		t.Fatalf("OpenHelp(2) = %d, want 2", len(got))
	}
	// Newest first.
	if got[0].Text != "need3" {
		t.Fatalf("OpenHelp order = %+v", got)
	}
}

// TestReplies_LimitTruncation exercises the limit truncation branch in Replies.
func TestReplies_LimitTruncation(t *testing.T) {
	st, _ := Open(t.TempDir())
	parent, _ := st.Post("t", "asker", "question", 1)
	st.Send(Message{Topic: "t", From: "x", Text: "r1", ReplyTo: parent.ID}, 2)
	st.Send(Message{Topic: "t", From: "y", Text: "r2", ReplyTo: parent.ID}, 3)
	st.Send(Message{Topic: "t", From: "z", Text: "r3", ReplyTo: parent.ID}, 4)
	got := st.Replies(parent.ID, 2)
	if len(got) != 2 {
		t.Fatalf("Replies(limit 2) = %d, want 2", len(got))
	}
	// Oldest first.
	if got[0].Text != "r1" {
		t.Fatalf("Replies order = %+v", got)
	}
}

// TestInbox_LimitTruncation exercises the limit truncation branch in Inbox.
func TestInbox_LimitTruncation(t *testing.T) {
	st, _ := Open(t.TempDir())
	st.Send(Message{Topic: "t", From: "a", To: "bob", Text: "m1"}, 1)
	st.Send(Message{Topic: "t", From: "a", To: "bob", Text: "m2"}, 2)
	st.Send(Message{Topic: "t", From: "a", To: "bob", Text: "m3"}, 3)
	got := st.Inbox("bob", 2, false)
	if len(got) != 2 {
		t.Fatalf("Inbox(limit 2) = %d, want 2", len(got))
	}
}

// TestSave_WriteError makes the tmp write target a directory so os.WriteFile
// fails inside save(), exercising the write-error return branch.
func TestSave_WriteError(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Occupy board.json.tmp with a directory so WriteFile can't create the file.
	if err := os.Mkdir(filepath.Join(dir, "board.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Post("t", "a", "x", 1); err == nil {
		t.Fatalf("Post: expected save write error")
	}
}
