// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
)

// The council's ports are EMPTY when there is no daemon.
//
// The council's ports are filled from the daemon's own kernel, by type: there
// is no foreign handle to fill them with any more. A host with no daemon
// holds no council: the seats are empty and the store is nowhere. The council
// answers with what it has rather than calling into nothing.
func TestTheCouncilPortsAreEmptyWithoutADaemon(t *testing.T) {
	ports := PortsFromKernel(nil, t.TempDir())

	if members := ports.Members(); len(members) != 0 {
		t.Errorf("members = %v, want none", members)
	}
	if ports.Store() != nil {
		t.Error("the council's ports hold a store on a host with no daemon")
	}
}

// The council's ENGINES refuse when the daemon holds no reasoning module.
//
// The engines are the REASONING MODULE's (W3.76v): the module keeps the
// council and the conductor, and the daemon holds the module's face. A daemon
// with no module holds no face, so the engines refuse — with the CONTRACT's
// row, so a caller compares with errors.Is and every layer says the same
// thing.
func TestTheCouncilEnginesRefuseWithoutTheReasoningModule(t *testing.T) {
	engines := EnginesFromKernel(nil)

	if _, err := engines.Convene(context.Background(), "corr", "why?", 1); !errors.Is(err, reasoningapi.ErrNoService) {
		t.Errorf("Convene error = %v, want the contract's refusal", err)
	}
	if _, err := engines.RunConductor(context.Background(), "corr", reasoningapi.ConductorConfig{Task: "x"}); !errors.Is(err, reasoningapi.ErrNoService) {
		t.Errorf("RunConductor error = %v, want the contract's refusal", err)
	}
}
