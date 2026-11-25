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
//
// This ratchet holds both directions, for every endpoint that takes a body.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/restapiapi"
)

// daemonRequestBodies are the daemon's own request structs, by the contract's
// endpoint id. An endpoint that takes a body MUST appear here: the ratchet
// cannot see a body the daemon decodes into a struct this table does not name.
func daemonRequestBodies() map[string]reflect.Type {
	return map[string]reflect.Type{
		restapiapi.EndpointRun:         reflect.TypeOf(runRequest{}),
		restapiapi.EndpointMailboxSend: reflect.TypeOf(mailboxSendRequest{}),
		restapiapi.EndpointMailboxAck:  reflect.TypeOf(mailboxAckRequest{}),
	}
}

// jsonFields reads the wire names of a struct's fields.
func jsonFields(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

// TestTheRequestRowsAgreeWithTheDaemonsRequestBodies is the ratchet: the
// contract's request rows and the daemon's request structs name the same
// fields.
func TestTheRequestRowsAgreeWithTheDaemonsRequestBodies(t *testing.T) {
	bodies := daemonRequestBodies()
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
		row, ok := restapiapi.RowByName(e.Request)
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
			if !read[f.Name] {
				t.Errorf("the contract's %q row names %q, which the %q endpoint does not read",
					row.Name, f.Name, e.ID)
			}
		}
	}
}
