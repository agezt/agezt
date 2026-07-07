// SPDX-License-Identifier: MIT

package builtinguardians

import (
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// errHost delegates to fakeHost but returns errors from configurable methods.
type errHost struct {
	fakeHost
	errOnAddAgent bool
}

func (h *errHost) AddAgent(_ roster.Profile) (roster.Profile, error) {
	if h.errOnAddAgent {
		return roster.Profile{}, errors.New("add agent error")
	}
	p := roster.Profile{Slug: "test", System: true}
	h.agents = append(h.agents, p)
	return p, nil
}

func (h *errHost) AddStanding(_ standing.Order) (standing.Order, error) {
	return standing.Order{}, errors.New("add standing error")
}

func (h *errHost) AddInterval(_ string, _ time.Duration, _, _ string) (cadence.Entry, error) {
	return cadence.Entry{}, errors.New("add interval error")
}

func (h *errHost) AddDaily(_ string, _ int, _, _ string) (cadence.Entry, error) {
	return cadence.Entry{}, errors.New("add daily error")
}

// TestSeedAll_AddAgentError checks that when AddAgent fails, firstErr is set
// and SeedAll continues to attempt remaining guardians.
func TestSeedAll_AddAgentError(t *testing.T) {
	h := &errHost{errOnAddAgent: true}
	seeded, err := SeedAll(h, "")
	if err == nil {
		t.Fatal("SeedAll should return an error when AddAgent fails")
	}
	if len(seeded) != 0 {
		t.Fatalf("SeedAll should return no seeded results on full failure, got %d", len(seeded))
	}
}

// TestSeedAll_SeedTriggerError checks that when seedTrigger fails (via
// AddStanding/AddInterval/AddDaily), the guardian is still listed as Created
// and the firstErr captures the trigger error.
func TestSeedAll_SeedTriggerError(t *testing.T) {
	h := &errHost{errOnAddAgent: false}
	seeded, err := SeedAll(h, "")
	if err == nil {
		t.Fatal("SeedAll should return an error when seedTrigger fails")
	}
	// Guardians are created even when their trigger fails.
	if len(seeded) != len(guardians) {
		t.Fatalf("SeedAll returned %d seeded, want %d (guardians created despite trigger errors)", len(seeded), len(guardians))
	}
	for _, s := range seeded {
		if !s.Created {
			t.Errorf("guardian %s should be Created=true despite trigger error", s.Slug)
		}
	}
}
