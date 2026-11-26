// SPDX-License-Identifier: MIT

package restapi

// The daemon's request bodies and the contract's request rows are one set.
//
// The REST contract (kernel/contract/restapiapi) names every field a request
// body carries, and the SDK catalogs render those rows, so an SDK sends a
// typed body. The daemon decodes its bodies into its OWN structs — so the
// daemon and the contract must read the SAME fields. Otherwise the vocabulary
// has two homes:
//
//   - a field the daemon reads and the contract does not name: the caller
//     cannot type it, no SDK can send it, and the contract cannot see it
//     (this is what `correlation_id` was — read by the daemon, invisible to
//     the contract, and so unsendable by any SDK);
//   - a field the contract names and the daemon ignores: the caller is told
//     it is accepted, and it is not.
//   - a field whose two types disagree: the door would measure the body
//     against a type the handler cannot hold.
//
// This ratchet holds both directions, for every endpoint that takes a body —
// the names AND the types.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
)

// daemonRequestBodies are the daemon's own request structs, by the contract's
// endpoint id. An endpoint that takes a body MUST appear here: the ratchet
// cannot see a body the daemon decodes into a struct this table does not name.
//
// The mailbox send body is the BOARD's wire row (kernel/contract/boardapi):
// the row lives in the contract, so the REST surface decodes it and the
// control plane's board_send operation reads the same one.
func daemonRequestBodies() map[string]reflect.Type {
	return map[string]reflect.Type{
		restapiapi.EndpointRun:         reflect.TypeOf(runRequest{}),
		restapiapi.EndpointMailboxSend: reflect.TypeOf(boardapi.Draft{}),
		restapiapi.EndpointMailboxAck:  reflect.TypeOf(boardapi.Ack{}),
		restapiapi.EndpointUpdateApply: reflect.TypeOf(updateApplyArgs{}),
	}
}

// requestRows returns the contract's request rows, by name.
func requestRows() map[string]restapiapi.Row {
	out := map[string]restapiapi.Row{}
	for _, r := range restapiapi.RequestRows() {
		out[r.Name] = r
	}
	return out
}

// jsonFields reads the wire names of a struct's fields, with their Go types.
func jsonFields(t reflect.Type) map[string]reflect.Type {
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

// goTypesFor is the Go types the daemon's struct may use for a wire type.
//
// The contract's row says what the field is on the wire; the daemon's struct
// says what it is in Go. A field whose two types disagree is a field the door
// would measure against a type the handler cannot hold — so the two must
// agree, not merely share a name.
func goTypesFor(t restapiapi.Type) []reflect.Type {
	switch t {
	case restapiapi.TypeString:
		return []reflect.Type{reflect.TypeOf("")}
	case restapiapi.TypeBool:
		return []reflect.Type{reflect.TypeOf(false)}
	case restapiapi.TypeInt:
		return []reflect.Type{
			reflect.TypeOf(int(0)), reflect.TypeOf(int64(0)), reflect.TypeOf(uint64(0)),
		}
	case restapiapi.TypeStringSlice:
		return []reflect.Type{reflect.TypeOf([]string{})}
	case restapiapi.TypeAnySlice:
		return []reflect.Type{reflect.TypeOf([]any{})}
	case restapiapi.TypeAnyObject:
		return []reflect.Type{reflect.TypeOf(map[string]any{})}
	case restapiapi.TypeIntMap:
		return []reflect.Type{reflect.TypeOf(map[string]int{})}
	default:
		return nil
	}
}

// typeAgrees reports whether the daemon's Go type is one the wire type allows.
func typeAgrees(wire restapiapi.Type, got reflect.Type) bool {
	for _, want := range goTypesFor(wire) {
		if got == want {
			return true
		}
	}
	return false
}

// TestTheRequestRowsAgreeWithTheDaemonsRequestBodies is the ratchet: the
// contract's request rows and the daemon's request structs name the same
// fields, with the same types.
func TestTheRequestRowsAgreeWithTheDaemonsRequestBodies(t *testing.T) {
	bodies := daemonRequestBodies()
	rows := requestRows()
	for _, e := range restapiapi.Endpoints() {
		if e.Request == "" {
			continue
		}
		typ, ok := bodies[e.ID]
		if !ok {
			t.Errorf("the %q endpoint takes the %q request body, but the daemon's request "+
				"struct for it is not in daemonRequestBodies() — the ratchet cannot see it",
				e.ID, e.Request)
			continue
		}
		row, ok := rows[e.Request]
		if !ok {
			t.Errorf("the %q endpoint takes the %q request body, which the contract has no row for",
				e.ID, e.Request)
			continue
		}
		read := jsonFields(typ)
		named := map[string]bool{}
		for _, f := range row.Fields {
			named[f.Name] = true
		}
		for name := range read {
			if !named[name] {
				t.Errorf("the %q endpoint reads %q, which the contract's %q row does not name",
					e.ID, name, row.Name)
			}
		}
		for _, f := range row.Fields {
			got, ok := read[f.Name]
			if !ok {
				t.Errorf("the contract's %q row names %q, which the %q endpoint does not read",
					row.Name, f.Name, e.ID)
				continue
			}
			if !typeAgrees(f.Type, got) {
				t.Errorf("the contract's %q row names %q as %q, the %q endpoint reads it as %s",
					row.Name, f.Name, f.Type, e.ID, got)
			}
		}
	}
}
