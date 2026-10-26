// SPDX-License-Identifier: MIT

package selfrepair

// Provenance: the self-repair module's own test for the board's ROW (W3.59).
//
//	The module used to name the board MODULE for one row — the message. The
//	row lives in the contract, so the module names the contract now, and the
//	edge from this module to the board module is gone.
//
//	The port is asserted at RUNTIME (the concrete store is held as the port
//	and driven through it), because a compile-time assertion would be a build
//	error, and a build error is not a test result.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/modules/board/board"
)

// TestTheMailboxSpeaksTheContractsRow: the self-repair mailbox port is
// served by the contract's store surface, and the row it carries is the
// contract's row.
func TestTheMailboxSpeaksTheContractsRow(t *testing.T) {
	store, err := board.Open(filepath.Join(t.TempDir(), "board"))
	if err != nil {
		t.Fatalf("open the board: %v", err)
	}
	var mailbox Mailbox = store
	if _, found := mailbox.Get("ghost"); found {
		t.Fatal("the mailbox invented a message")
	}
	msg, err := mailbox.HelpRequest("repairer", "operator", "needs a hand", 100)
	if err != nil {
		t.Fatalf("HelpRequest: %v", err)
	}
	var row boardapi.Message = msg
	if row.Topic != "help" || !row.Help || row.From != "repairer" {
		t.Fatalf("the mailbox row is wrong: %+v", row)
	}
}
