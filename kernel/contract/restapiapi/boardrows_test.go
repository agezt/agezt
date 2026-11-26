// SPDX-License-Identifier: MIT

package restapiapi

// The board's mail row has one home.
//
// The REST contract names the mail row the SDKs speak, and the daemon serves
// the board's wire row — which lives in the BOARD contract
// (kernel/contract/boardapi), because three surfaces serve it and none of
// them may spell its field names a second time.
//
// The two are related by the contract's own rule: a RESPONSE row is the STABLE
// SUBSET of the payload. The daemon may add a field the SDKs do not name
// (`acked_by` is one — the server may add fields to a response), so the
// subset direction is the one that binds:
//
//   - every field the contract names must be on the wire row — a renamed wire
//     field would leave the SDKs reading a field the daemon no longer sends;
//   - the wire row may carry MORE — that is the rule, not a drift.
//
// The types are checked too: a field the contract calls a string and the wire
// carries as a number is a field the SDK would misread.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// wireNames reads the wire names of a struct's fields.
func wireNames(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = t.Field(i).Type
	}
	return out
}

// goTypesFor is the Go types a wire type may be read as.
func goTypesFor(t Type) []reflect.Type {
	switch t {
	case TypeString:
		return []reflect.Type{reflect.TypeOf("")}
	case TypeBool:
		return []reflect.Type{reflect.TypeOf(false)}
	case TypeInt:
		return []reflect.Type{
			reflect.TypeOf(int(0)), reflect.TypeOf(int64(0)), reflect.TypeOf(uint64(0)),
		}
	case TypeStringSlice:
		return []reflect.Type{reflect.TypeOf([]string{})}
	case TypeAnySlice:
		return []reflect.Type{reflect.TypeOf([]any{})}
	case TypeAnyObject:
		return []reflect.Type{reflect.TypeOf(map[string]any{})}
	case TypeIntMap:
		return []reflect.Type{reflect.TypeOf(map[string]int{})}
	case TypeMail:
		return []reflect.Type{reflect.TypeOf(boardapi.Mail{})}
	case TypeMailSlice:
		return []reflect.Type{reflect.TypeOf([]boardapi.Mail{})}
	default:
		return nil
	}
}

// typeAgrees reports whether the wire's Go type is one the contract's type
// allows.
func typeAgrees(wire Type, got reflect.Type) bool {
	for _, want := range goTypesFor(wire) {
		if got == want {
			return true
		}
	}
	// A free-form array is served by ANY slice: the contract says the values
	// are free-form, not that the array holds one particular Go type.
	if wire == TypeAnySlice && got.Kind() == reflect.Slice {
		return true
	}
	// A field the contract calls a string may be carried as a string OR as a
	// pointer to one: the pointer is how a row says "absent" (the run row's
	// answer is present in the successful answer and absent in the failed
	// one).
	if wire == TypeString && got.Kind() == reflect.Ptr {
		return got.Elem() == reflect.TypeOf("")
	}
	return false
}

// checkSubset holds one contract row to one served row: every field the
// contract names must be on the served row, under the same name and a type the
// contract allows.
func checkSubset(t *testing.T, contractRow string, sent reflect.Type) {
	t.Helper()
	row := rowByName(t, contractRow)
	wire := wireNames(sent)
	for _, f := range row.Fields {
		got, ok := wire[f.Name]
		if !ok {
			t.Errorf("the contract's %q row names %q, which the row the daemon serves does not carry",
				row.Name, f.Name)
			continue
		}
		if !typeAgrees(f.Type, got) {
			t.Errorf("the contract's %q row names %q as %q, the row the daemon serves carries it as %s",
				row.Name, f.Name, f.Type, got)
		}
	}
}

// TestTheMailRowIsAStableSubsetOfTheBoardsWireRow is the ratchet: every field
// the contract's mail row names must be a field of the board's wire row, under
// the same name and a type the contract allows.
func TestTheMailRowIsAStableSubsetOfTheBoardsWireRow(t *testing.T) {
	checkSubset(t, RowMail, reflect.TypeOf(boardapi.Mail{}))
}

// envelopeRows are the answers the mailbox surface sends around the messages:
// the list, the send's answer, the replies, the inbox, the topics and the
// ack. Each one lives in the board contract, for the same reason the mail row
// does — the surface sends it, the contract names it, and the SDKs read it.
//
// The table is the slice's coverage claim: a row the surface sends and the
// table does not name is a row nobody checks.
var envelopeRows = []struct {
	ContractRow string
	Sent        reflect.Type
}{
	{RowMailboxList, reflect.TypeOf(boardapi.MailboxList{})},
	{RowMailboxSent, reflect.TypeOf(boardapi.MailboxSent{})},
	{RowMailboxReplies, reflect.TypeOf(boardapi.MailboxReplies{})},
	{RowMailboxInbox, reflect.TypeOf(boardapi.MailboxInbox{})},
	{RowMailboxTopics, reflect.TypeOf(boardapi.MailboxTopics{})},
	{RowMailAcked, reflect.TypeOf(boardapi.MailAcked{})},
}

// TestTheEnvelopeRowsAreTheStableSubsetOfTheRowsTheDaemonSends is the same
// ratchet for the answers around the messages.
func TestTheEnvelopeRowsAreTheStableSubsetOfTheRowsTheDaemonSends(t *testing.T) {
	for _, entry := range envelopeRows {
		checkSubset(t, entry.ContractRow, entry.Sent)
	}
}
