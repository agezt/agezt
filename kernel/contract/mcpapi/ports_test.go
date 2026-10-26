// SPDX-License-Identifier: MIT

package mcpapi_test

// Provenance: the contract's own test for the MCP ports (W3.58). The port
//
//	shapes are the contract's: the layers above the module speak them, so a
//	port that grew a verb would hand a caller a capability nobody asked for,
//	and a port that lost one would break a caller that had it.
//
//	The WIDTH of each port is asserted here — by name, not by behaviour — so
//	a change to the surface is a deliberate act with a failing test beside it.

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
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

// requirePort asserts the port carries exactly the wanted verbs.
func requirePort(t *testing.T, port reflect.Type, want ...string) {
	t.Helper()
	got := portVerbs(t, port)
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

// TestRegistryPortCarriesTheWholeRegistryAndNothingMore: the registry's port
// is the store's whole surface — the lifecycle (add, enable, remove) and the
// reads (get, list, count).
//
// The count is ONCE: the registry is a live file, and a caller that wanted
// both "the rows" and "how many rows" would be able to ask the same question
// twice and get two answers.
func TestRegistryPortCarriesTheWholeRegistryAndNothingMore(t *testing.T) {
	requirePort(t, reflect.TypeFor[mcpapi.Registry](),
		"Add", "Count", "Get", "List", "Remove", "SetEnabled")
}

// TestRegistrationReaderPortListsAndNothingElse: the catalog only LISTS. A
// reader that could also add or remove a server would hand the catalog the
// lifecycle it has no business driving.
func TestRegistrationReaderPortListsAndNothingElse(t *testing.T) {
	requirePort(t, reflect.TypeFor[mcpapi.RegistrationReader](), "List")
}

// TestConnPortIsAToolsCallCloseTriple: a live attachment is exactly three
// verbs — what it offers, how to call it, how to let it go.
func TestConnPortIsAToolsCallCloseTriple(t *testing.T) {
	requirePort(t, reflect.TypeFor[mcpapi.Conn](), "Call", "Close", "Tools")
}

// TestDialerSeamsCarryTheTransportSignatures: the dialers are SEAMS — the
// host injects the implementation, so the kernel never names the module's
// client. The stdio seam spawns a process; the http seam speaks to a URL.
func TestDialerSeamsCarryTheTransportSignatures(t *testing.T) {
	stdio := reflect.TypeFor[mcpapi.Dialer]()
	if stdio.Kind() != reflect.Func {
		t.Fatalf("the stdio dialer is not a func")
	}
	if stdio.NumIn() != 4 || stdio.NumOut() != 2 {
		t.Fatalf("the stdio dialer takes %d args and returns %d", stdio.NumIn(), stdio.NumOut())
	}
	if stdio.In(0) != reflect.TypeFor[context.Context]() || stdio.In(1) != reflect.TypeFor[string]() ||
		stdio.In(2) != reflect.TypeFor[[]string]() || stdio.In(3) != reflect.TypeFor[map[string]string]() {
		t.Fatalf("the stdio dialer's arguments drifted: %v", stdio.In(0))
	}
	if stdio.Out(0) != reflect.TypeFor[mcpapi.Conn]() || stdio.Out(1) != reflect.TypeFor[error]() {
		t.Fatalf("the stdio dialer's results drifted: %v -> %v", stdio.Out(0), stdio.Out(1))
	}

	remote := reflect.TypeFor[mcpapi.HTTPDialer]()
	if remote.Kind() != reflect.Func {
		t.Fatalf("the http dialer is not a func")
	}
	if remote.NumIn() != 3 || remote.NumOut() != 2 {
		t.Fatalf("the http dialer takes %d args and returns %d", remote.NumIn(), remote.NumOut())
	}
	if remote.In(0) != reflect.TypeFor[context.Context]() || remote.In(1) != reflect.TypeFor[string]() ||
		remote.In(2) != reflect.TypeFor[map[string]string]() {
		t.Fatalf("the http dialer's arguments drifted: %v", remote.In(0))
	}
	if remote.Out(0) != reflect.TypeFor[mcpapi.Conn]() || remote.Out(1) != reflect.TypeFor[error]() {
		t.Fatalf("the http dialer's results drifted: %v -> %v", remote.Out(0), remote.Out(1))
	}
}

// TestErrNotFoundIsTheContractSentinel: the sentinel lives in the contract
// because the layers ABOVE the module compare against it. A module that
// raised a look-alike error of its own would leave every errors.Is
// comparison silently false — and the app's "unknown mcp server" would never
// be reached.
func TestErrNotFoundIsTheContractSentinel(t *testing.T) {
	if mcpapi.ErrNotFound == nil {
		t.Fatal("the contract has no not-found sentinel")
	}
	if mcpapi.ErrNotFound.Error() != "mcp: server not found" {
		t.Fatalf("the sentinel's message drifted: %q", mcpapi.ErrNotFound.Error())
	}
	// A DIFFERENT sentinel with the same message would break errors.Is.
	if !errors.Is(mcpapi.ErrNotFound, mcpapi.ErrNotFound) {
		t.Fatal("the sentinel does not match itself")
	}
	if errors.Is(errors.New("mcp: server not found"), mcpapi.ErrNotFound) {
		t.Fatal("a look-alike error with the same message matched the sentinel")
	}
}

// TestToolDefIsTheAdvertisedToolRow: the row a server advertises. Its shape
// is the wire shape — a name, a description and an input schema.
func TestToolDefIsTheAdvertisedToolRow(t *testing.T) {
	typ := reflect.TypeFor[mcpapi.ToolDef]()
	if typ.NumField() != 3 {
		t.Fatalf("the tool row carries %d fields, want 3", typ.NumField())
	}
	for _, want := range []struct{ name, tag string }{
		{"Name", "name"},
		{"Description", "description,omitempty"},
		{"InputSchema", "inputSchema,omitempty"},
	} {
		field, ok := typ.FieldByName(want.name)
		if !ok {
			t.Fatalf("the tool row has no %s field", want.name)
		}
		if got := field.Tag.Get("json"); got != want.tag {
			t.Fatalf("the tool row's %s json tag is %q, want %q", want.name, got, want.tag)
		}
	}
	// The schema travels as raw JSON — the contract must not re-parse it.
	raw, err := json.Marshal(mcpapi.ToolDef{Name: "greet", Description: "greets", InputSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"name":"greet","description":"greets","inputSchema":{"type":"object"}}` {
		t.Fatalf("the tool row's wire shape drifted: %s", raw)
	}
}
