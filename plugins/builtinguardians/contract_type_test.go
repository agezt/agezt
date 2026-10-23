// SPDX-License-Identifier: MIT

package builtinguardians

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/guardianapi"
)

const (
	contractPkg = "github.com/agezt/agezt/kernel/contract/guardianapi"
	rosterPkg   = "github.com/agezt/agezt/kernel/modules/agents/roster"
)

// TestTheSeederNamesTheContractPort: the seeder is a PLUGIN, so it may reach
// the contract layer and nothing else. Its host is the contract's port, and
// the rows it writes are the contract's rows — the roster module keeps
// aliases of them, but a plugin may not name a module.
func TestTheSeederNamesTheContractPort(t *testing.T) {
	hostType := reflect.TypeOf((*Host)(nil)).Elem()
	if hostType.PkgPath() != contractPkg {
		t.Fatalf("the seeder's host must come from %s, got %s", contractPkg, hostType.PkgPath())
	}
	if hostType != reflect.TypeOf((*guardianapi.Host)(nil)).Elem() {
		t.Fatal("the seeder's host must be the contract's port")
	}
}

// TestTheSeederWritesTheContractRows: the agent profile and the noise policy
// the seeder writes are the CONTRACT's rows. The roster module's `Profile`
// and `NoisePolicy` are aliases of these — so a drift in either would show up
// here as a different type.
func TestTheSeederWritesTheContractRows(t *testing.T) {
	policy := defaultGuardianNoisePolicy()
	if policy == nil {
		t.Fatal("the default noise policy must not be nil")
	}
	var contract agentapi.NoisePolicy = *policy
	if contract.SilentOnSuccess != policy.SilentOnSuccess {
		t.Fatalf("the noise policy did not survive the crossing: %+v", contract)
	}
	if reflect.TypeOf(*policy) != reflect.TypeOf(agentapi.NoisePolicy{}) {
		t.Fatalf("the noise policy is %v, want the contract's", reflect.TypeOf(*policy))
	}
}

// TestTheSeederDoesNotNameTheRosterModule: the plugin may not name the roster
// module — the archcheck gate enforces it, and this test names the rule so a
// future reader knows where the boundary is.
func TestTheSeederDoesNotNameTheRosterModule(t *testing.T) {
	// The seeder's own surface must not carry the module's package.
	hostType := reflect.TypeOf((*Host)(nil)).Elem()
	for i := 0; i < hostType.NumMethod(); i++ {
		m := hostType.Method(i)
		for j := 0; j < m.Type.NumIn(); j++ {
			if m.Type.In(j).PkgPath() == rosterPkg {
				t.Errorf("the seeder's port names the roster module: %s", m.Name)
			}
		}
		for j := 0; j < m.Type.NumOut(); j++ {
			if m.Type.Out(j).PkgPath() == rosterPkg {
				t.Errorf("the seeder's port names the roster module: %s", m.Name)
			}
		}
	}
}
