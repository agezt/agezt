// SPDX-License-Identifier: MIT

package boardapi_test

// Provenance: the contract's own test for the board's PORT (W3.59). The
//
//	board's port is the contract's — the layers above the module speak it —
//	and a port that grew a verb would hand a caller a capability nobody
//	asked for.
//
//	The WIDTH of the port is asserted here — by name, not by behaviour — so a
//	change to the surface is a deliberate act with a failing test beside it.

import (
	"reflect"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// portVerbs lists an interface port's verbs — the port's WIDTH.
func portVerbs(t *testing.T, port reflect.Type) []string {
	t.Helper()
	if port.Kind() != reflect.Interface {
		t.Fatalf("%s is not an interface", port)
	}
	names := make([]string, 0, port.NumMethod())
	for i := 0; i < port.NumMethod(); i++ {
		names = append(names, port.Method(i).Name)
	}
	sort.Strings(names)
	return names
}

// TestStorePortCarriesTheWholeBoardAndNothingMore: the board's port is the
// store's whole surface — the eleven verbs the board has: the four writes
// (post, send, broadcast, help request), the ack, and the six reads.
//
// The port is NOT widened for the convenience of a caller: the board tool
// (a plugin) takes this same port, and a port that carried the file's
// lifecycle would hand a plugin the ability to own the board.
func TestStorePortCarriesTheWholeBoardAndNothingMore(t *testing.T) {
	got := portVerbs(t, reflect.TypeFor[boardapi.Store]())
	want := []string{
		"Ack", "Broadcast", "Get", "HelpRequest", "Inbox",
		"OpenHelp", "Post", "Read", "Replies", "Send", "Topics",
	}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("the port carries %v, want exactly %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("the port carries %v, want exactly %v", got, want)
		}
	}
}

// TestEveryoneIsTheWildcardRecipient: the wildcard is a VALUE the board file
// stores and every reader compares against, so it is pinned here rather than
// repeated as a literal at each reader.
func TestEveryoneIsTheWildcardRecipient(t *testing.T) {
	if boardapi.Everyone != "*" {
		t.Fatalf("the wildcard recipient is %q, want %q", boardapi.Everyone, "*")
	}
}
