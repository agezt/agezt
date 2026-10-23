// SPDX-License-Identifier: MIT

package guardianapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
)

// TestThePortHasExactlyElevenMethods: the seeder's port is the whole of what
// a plugin may ask of the live daemon — the roster, the standing orders and
// the schedules, and nothing else. A twelfth method would be a new reach
// into the daemon, and it would have to be earned here.
func TestThePortHasExactlyElevenMethods(t *testing.T) {
	typ := reflect.TypeOf((*Host)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Host must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 11 {
		t.Fatalf("the port has %d methods, want 11: %v", typ.NumMethod(), methodNames(typ))
	}
}

// TestThePortCarriesTheContractRows: the rows the port carries are the
// CONTRACT's own — the agent profile and its noise policy live in
// `agentapi`, the standing order in `standingapi` and the schedule in
// `cadenceapi`. The roster module keeps aliases of the first two, but a
// plugin may not name a module.
func TestThePortCarriesTheContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Host)(nil)).Elem()

	for _, c := range []struct {
		method string
		want   reflect.Type
	}{
		{"AddAgent", reflect.TypeOf(agentapi.Profile{})},
		{"UpdateAgent", reflect.TypeOf(agentapi.Profile{})},
		{"AddStanding", reflect.TypeOf(standingapi.Order{})},
		{"UpdateStanding", reflect.TypeOf(standingapi.Order{})},
		{"SetStandingEnabled", reflect.TypeOf(standingapi.Order{})},
		{"AddInterval", reflect.TypeOf(cadenceapi.Entry{})},
		{"AddDaily", reflect.TypeOf(cadenceapi.Entry{})},
	} {
		m, ok := typ.MethodByName(c.method)
		if !ok {
			t.Fatalf("the port must have a %s method", c.method)
		}
		found := false
		for i := 0; i < m.Type.NumOut(); i++ {
			if m.Type.Out(i) == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s must answer %v, got %v", c.method, c.want, m.Type)
		}
	}

	// The mutators take the contract's rows too.
	update, _ := typ.MethodByName("UpdateAgent")
	if update.Type.In(1) != reflect.TypeOf(func(*agentapi.Profile) {}) {
		t.Errorf("UpdateAgent must take the contract's mutator, got %v", update.Type.In(1))
	}
	standing, _ := typ.MethodByName("UpdateStanding")
	if standing.Type.In(1) != reflect.TypeOf(func(*standingapi.Order) {}) {
		t.Errorf("UpdateStanding must take the contract's mutator, got %v", standing.Type.In(1))
	}

	// The schedule cadence is a plain duration — the port must not carry a
	// module's row for it.
	reschedule, _ := typ.MethodByName("Reschedule")
	if reschedule.Type.In(2) != reflect.TypeOf(time.Duration(0)) {
		t.Errorf("Reschedule must take a duration, got %v", reschedule.Type.In(2))
	}
}

// methodNames lists the port's methods — for the failure messages.
func methodNames(typ reflect.Type) []string {
	out := make([]string, 0, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		out = append(out, typ.Method(i).Name)
	}
	return out
}
