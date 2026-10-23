// SPDX-License-Identifier: MIT

package boardtool

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// contractRowKeys reads the board message row's own wire keys from the
// contract row's tags — so the tool's view is checked against the DECLARATION
// and not against a copy of it that could drift with it.
func contractRowKeys(t *testing.T) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(boardapi.Message{})
	keys := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		name := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
	}
	return keys
}

// TestTheMessageViewIsTheContractRow: the tool's view must carry the board
// row's own values under the board row's own key names, and the key set is
// pinned. The keys are read BY NAME (the model reads them), so a renamed key
// is a silent break for every consumer outside this package — and no test in
// this package would notice, because the package writes and reads the same
// map.
func TestTheMessageViewIsTheContractRow(t *testing.T) {
	m := boardapi.Message{
		ID: "m1", Topic: "help", From: "alice", To: "bob", ReplyTo: "m0",
		Text: "please help", Help: true, TSMS: 1700000000000, AckedBy: []string{"carol"},
	}
	view := msgView(m)

	want := []string{"acked_by", "at", "from", "help", "id", "reply_to", "text", "to", "topic"}
	for _, k := range want {
		if _, ok := view[k]; !ok {
			t.Errorf("the view lost the key %q: %+v", k, view)
		}
	}
	if len(view) != len(want) {
		t.Errorf("the view grew a key: %+v", view)
	}

	// Every key but the tool's own rendering of the timestamp is a key of the
	// contract row.
	keys := contractRowKeys(t)
	for k := range view {
		if k == "at" {
			continue // the tool renders ts_ms as an RFC3339 instant
		}
		if !keys[k] {
			t.Errorf("the key %q is not a key of the contract row", k)
		}
	}

	// The values must be the row's own.
	if view["id"] != "m1" || view["topic"] != "help" || view["from"] != "alice" ||
		view["to"] != "bob" || view["reply_to"] != "m0" || view["text"] != "please help" {
		t.Fatalf("the view does not carry the message's own values: %+v", view)
	}
	if view["help"] != true {
		t.Errorf("help = %v, want true", view["help"])
	}
	acked, ok := view["acked_by"].([]string)
	if !ok || len(acked) != 1 || acked[0] != "carol" {
		t.Errorf("acked_by = %v, want [carol]", view["acked_by"])
	}
	if view["at"] != time.UnixMilli(m.TSMS).Format(time.RFC3339) {
		t.Errorf("at = %v, want the message's own instant", view["at"])
	}
}

// TestTheMessageViewOmitsTheEmptyFields: an empty field must stay out of the
// wire — a key with an empty value reads as "there is one, and it is empty".
func TestTheMessageViewOmitsTheEmptyFields(t *testing.T) {
	view := msgView(boardapi.Message{Topic: "t", Text: "x"})
	want := []string{"topic", "text"}
	if len(view) != len(want) {
		t.Fatalf("view = %+v, want only %v", view, want)
	}
	for _, k := range want {
		if _, ok := view[k]; !ok {
			t.Errorf("the view lost the key %q: %+v", k, view)
		}
	}
}
