// SPDX-License-Identifier: MIT

package skill

import (
	"sort"
	"testing"
)

// putActive seeds one ACTIVE skill directly into the forge's store with an
// owner — the retrieval-pool shape ActivateFor filters (M932).
func putActive(t *testing.T, f *Forge, name, agent string) {
	t.Helper()
	sk := Skill{
		ID:          ContentID(name, "body of "+name),
		Name:        name,
		Description: "deploy procedure " + name,
		Body:        "body of " + name,
		Status:      StatusActive,
		Agent:       agent,
		Version:     DefaultVersion,
		CreatedMS:   fixedNow.UnixMilli(),
		LastSeenMS:  fixedNow.UnixMilli(),
	}
	if err := f.store.Put(sk); err != nil {
		t.Fatalf("Put(%s): %v", name, err)
	}
}

func hitNames(hits []Scored) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Skill.Name)
	}
	sort.Strings(out)
	return out
}

// TestActivateFor_AgentScopeWall — the per-agent skill wall (M932): an agent
// retrieves the shared pool plus its own private skills, never a sibling's;
// the default persona (no agent) retrieves only the shared pool.
func TestActivateFor_AgentScopeWall(t *testing.T) {
	f, _ := newTestForge(t)
	putActive(t, f, "shared-proc", "")
	putActive(t, f, "alpha-proc", "alpha")
	putActive(t, f, "beta-proc", "beta")

	intent := "run the deploy procedure"

	hits, err := f.ActivateFor("c1", "alpha", intent, 10)
	if err != nil {
		t.Fatalf("ActivateFor(alpha): %v", err)
	}
	if got := hitNames(hits); len(got) != 2 || got[0] != "alpha-proc" || got[1] != "shared-proc" {
		t.Errorf("alpha pool = %v, want [alpha-proc shared-proc]", got)
	}

	hits, err = f.Activate("c2", intent, 10)
	if err != nil {
		t.Fatalf("Activate (default persona): %v", err)
	}
	if got := hitNames(hits); len(got) != 1 || got[0] != "shared-proc" {
		t.Errorf("default-persona pool = %v, want [shared-proc] only", got)
	}
}

// TestCreate_StampsOwningAgent — a created skill records its owner and the
// skill.created event carries it; an unowned create stays shared.
func TestCreate_StampsOwningAgent(t *testing.T) {
	f, _ := newTestForge(t)
	sk, created, err := f.Create("c1", CreateSpec{Name: "private-trick", Body: "steps", Agent: "alpha"})
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if sk.Agent != "alpha" {
		t.Errorf("Agent = %q, want alpha", sk.Agent)
	}
	shared, created, err := f.Create("c2", CreateSpec{Name: "common-trick", Body: "steps"})
	if err != nil || !created {
		t.Fatalf("create shared: created=%v err=%v", created, err)
	}
	if shared.Agent != "" {
		t.Errorf("shared Agent = %q, want empty", shared.Agent)
	}
}
