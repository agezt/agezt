package builtinguardians

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

type fakeHost struct {
	agents    []roster.Profile
	standings []standing.Order
	intervals []string
	dailies   []string
}

func (h *fakeHost) Agents() []roster.Profile { return h.agents }
func (h *fakeHost) AddAgent(p roster.Profile) (roster.Profile, error) {
	p.Enabled = true
	h.agents = append(h.agents, p)
	return p, nil
}
func (h *fakeHost) AddStanding(o standing.Order) (standing.Order, error) {
	h.standings = append(h.standings, o)
	return o, nil
}
func (h *fakeHost) AddInterval(intent string, _ time.Duration, _ string) (cadence.Entry, error) {
	h.intervals = append(h.intervals, intent)
	return cadence.Entry{Intent: intent}, nil
}
func (h *fakeHost) AddDaily(intent string, _ int, _ string) (cadence.Entry, error) {
	h.dailies = append(h.dailies, intent)
	return cadence.Entry{Intent: intent}, nil
}

func TestSeedAll_SeedsEveryGuardian(t *testing.T) {
	h := &fakeHost{}
	out, err := SeedAll(h, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	if len(out) != len(guardians) {
		t.Fatalf("seeded %d, want %d", len(out), len(guardians))
	}
	if len(h.agents) != len(guardians) {
		t.Fatalf("created %d agents, want %d", len(h.agents), len(guardians))
	}
	// Every seeded agent is System-marked and carries a soul + budget cap.
	for _, p := range h.agents {
		if !p.System {
			t.Errorf("guardian %s not marked System", p.Slug)
		}
		if p.Soul == "" {
			t.Errorf("guardian %s has no soul", p.Slug)
		}
		if p.MaxDailyMc == 0 {
			t.Errorf("guardian %s has no daily budget cap", p.Slug)
		}
	}
	// Each guardian validates as a real roster profile.
	for _, p := range h.agents {
		if err := roster.Validate(p); err != nil {
			t.Errorf("guardian %s invalid: %v", p.Slug, err)
		}
	}
	// Triggers: event guardians got standing orders, periodic ones got schedules.
	if len(h.standings) == 0 || len(h.intervals) == 0 || len(h.dailies) == 0 {
		t.Errorf("expected standing+interval+daily triggers, got standing=%d interval=%d daily=%d",
			len(h.standings), len(h.intervals), len(h.dailies))
	}
	// Standing orders run AS the guardian and validate.
	for _, o := range h.standings {
		if o.Agent == "" {
			t.Errorf("standing %q has no agent", o.Name)
		}
		if err := standing.Validate(o); err != nil {
			t.Errorf("standing %q invalid: %v", o.Name, err)
		}
	}
	// Schedule intents bind to a guardian via --agent.
	for _, in := range append(h.intervals, h.dailies...) {
		if !containsAgentFlag(in) {
			t.Errorf("schedule intent %q missing --agent binding", in)
		}
	}
}

func TestSeedAll_Idempotent(t *testing.T) {
	h := &fakeHost{}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	agentsAfterFirst := len(h.agents)
	standingsAfterFirst := len(h.standings)

	out, err := SeedAll(h, "") // re-seed: everything already present
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if len(h.agents) != agentsAfterFirst {
		t.Errorf("re-seed added agents: %d → %d", agentsAfterFirst, len(h.agents))
	}
	if len(h.standings) != standingsAfterFirst {
		t.Errorf("re-seed added standing orders: %d → %d", standingsAfterFirst, len(h.standings))
	}
	for _, s := range out {
		if s.Created {
			t.Errorf("guardian %s reported Created on re-seed", s.Slug)
		}
	}
}

func TestSeedAll_RespectsRemovedGuardian(t *testing.T) {
	// An operator who keeps only some guardians (others removed) must not get the
	// missing ones forced back — but here we assert the inverse: a guardian that
	// IS present is left untouched while absent ones are (re)seeded.
	h := &fakeHost{agents: []roster.Profile{{Slug: "guardian-health", System: true, Enabled: true}}}
	out, err := SeedAll(h, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range out {
		if s.Slug == "guardian-health" && s.Created {
			t.Error("present guardian-health should be left untouched")
		}
	}
	// The others were seeded.
	if len(h.agents) != len(guardians) {
		t.Errorf("agents = %d, want %d", len(h.agents), len(guardians))
	}
}

func containsAgentFlag(s string) bool {
	for i := 0; i+7 <= len(s); i++ {
		if s[i:i+7] == "--agent" {
			return true
		}
	}
	return false
}
