// SPDX-License-Identifier: MIT

package builtinguardians

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// TestSeedAllRefusesAMissingHost: the host is the contract's port, and a
// daemon that handed over nothing must hear that the fleet was not seeded —
// not a panic on the first read, and not a silent success.
func TestSeedAllRefusesAMissingHost(t *testing.T) {
	seeded, err := SeedAll(nil, "")
	if err == nil {
		t.Fatalf("a missing host must be refused, got %v", seeded)
	}
	if !strings.Contains(err.Error(), "live daemon") {
		t.Fatalf("the refusal must name what is missing, got %q", err.Error())
	}
	if len(seeded) != 0 {
		t.Fatalf("a refused seed must report nothing, got %v", seeded)
	}
}

// TestTheDefaultNoisePolicyIsQuiet: the shipped guardians are quiet by
// default — routine green runs finish without interrupting the operator, and
// they do not write memory. The values are the contract with the operator, so
// they are pinned here.
func TestTheDefaultNoisePolicyIsQuiet(t *testing.T) {
	policy := defaultGuardianNoisePolicy()
	if policy == nil {
		t.Fatal("the default noise policy must not be nil")
	}
	if !policy.SilentOnSuccess {
		t.Error("the guardians must be silent on success by default")
	}
	if !policy.DisableMemoryWrites {
		t.Error("the guardians must not write memory by default")
	}
}

// TestTheSeededProfileCarriesTheContractRows: the profile the seeder writes
// is the contract's row — the agent profile and its noise policy both live in
// `agentapi` now.
func TestTheSeededProfileCarriesTheContractRows(t *testing.T) {
	host := &fakeHost{}
	seeded, err := SeedAll(host, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	if len(seeded) == 0 {
		t.Fatal("the seeder must seed the guardians")
	}
	for _, p := range host.agents {
		var contract agentapi.Profile = p
		if contract.Slug == "" {
			t.Fatalf("a seeded agent has no slug: %+v", contract)
		}
	}
}
