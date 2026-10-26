// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the MESSAGE BOARD (W3.59). The
//
//	board's FILE is the daemon's: the daemon opens it once and hands every
//	holder the contract's port — the control plane, the REST mailbox,
//	self-repair and the board tool all speak `boardapi.Store` and none of them
//	may name the module.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// TestTheDaemonOpensTheBoardsFile: the board is a FILE on disk under the
// base dir, and the daemon hands the port over — not the concrete store.
func TestTheDaemonOpensTheBoardsFile(t *testing.T) {
	dir := t.TempDir()
	store, err := openBoard(dir)
	if err != nil {
		t.Fatalf("open the board: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "board", "board.json")); err == nil {
		t.Fatal("a board nobody wrote to yet has no file")
	}
	var port boardapi.Store = boardPort(store)
	if port == nil {
		t.Fatal("the daemon must hand the board over as a live port")
	}
	// The file appears with the first write — the store holds the whole list
	// in memory and saves it whole.
	if _, err := port.Post("status", "tester", "hello", 100); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "board", "board.json")); err != nil {
		t.Fatalf("the daemon's board must be a file on disk: %v", err)
	}
	if _, found := port.Get("ghost"); found {
		t.Fatal("a fresh board invented a message")
	}
}

// TestTheBoardsNilPortRule: the board store is opened LATE and the daemon
// leaves it nil when it could not be opened. A nil *board.Store handed to an
// interface parameter is a NON-NIL interface — so the conversion has to happen
// where the value is still concrete, and a board that is not there must stay
// a nil PORT.
//
// This is not theoretical: self-repair's mailbox port took the concrete store
// before W3.59, and its nil check would not have fired.
func TestTheBoardsNilPortRule(t *testing.T) {
	if got := boardPort(nil); got != nil {
		t.Fatal("a board that could not be opened must stay a nil PORT")
	}
}
