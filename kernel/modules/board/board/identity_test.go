// SPDX-License-Identifier: MIT

package board

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// The board's row and the contract's row must be the SAME type.
//
// The host's methods speak the board's name and the tools that read the board
// speak the contract's (the board tool and the overseer tool are plugins, and
// a plugin may only reach the contract). The wiring only compiles when the
// two are ONE type — so if the board ever declares its own copy of the row,
// the break shows up at the far end of the tree.
//
// The function-type form is deliberate: an ASSIGNMENT would also accept a copy
// with the same fields, but a function type requires IDENTITY.
var _ func(boardapi.Message) string = func(m boardapi.Message) string { return m.Topic }

// TestMessageWireShape pins the row's wire keys: the Web UI, the SDK and the
// persisted board file all read the message by KEY NAME, so a renamed key is a
// silent break for every consumer that does not live in this package.
func TestMessageWireShape(t *testing.T) {
	full := Message{
		ID:      "m1",
		Topic:   "help",
		From:    "a",
		To:      "b",
		ReplyTo: "m0",
		Text:    "please help",
		Help:    true,
		TSMS:    42,
		AckedBy: []string{"c"},
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "topic", "from", "to", "reply_to", "text", "help", "ts_ms", "acked_by"} {
		if _, ok := out[key]; !ok {
			t.Errorf("the message's wire is missing %q", key)
		}
	}
	if _, ok := out["replyTo"]; ok {
		t.Error("the message's wire grew a camelCase key beside reply_to")
	}
}

// TestTheRowIsTheContractRow states the same fact at runtime: the board's row
// and the contract's row are interchangeable, field for field.
func TestTheRowIsTheContractRow(t *testing.T) {
	m := Message{
		ID:      "m1",
		Topic:   "help",
		From:    "a",
		To:      "b",
		ReplyTo: "m0",
		Text:    "please help",
		Help:    true,
		TSMS:    42,
		AckedBy: []string{"c"},
	}
	var contract boardapi.Message = m
	if contract.Topic != "help" || contract.Text != "please help" || !contract.Help {
		t.Fatalf("the message did not survive the crossing: %+v", contract)
	}
	if contract.TSMS != 42 || len(contract.AckedBy) != 1 || contract.AckedBy[0] != "c" {
		t.Fatalf("the message did not survive the crossing: %+v", contract)
	}
}
