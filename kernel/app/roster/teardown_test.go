// SPDX-License-Identifier: MIT

package roster

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
	core "github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// teardownFake is a host stand-in for the teardown ports.
type teardownFake struct {
	orders   []standing.Order
	entries  []cadence.Entry
	retired  []string
	reasons  []string
	paused   []string
	removed  []string
	missing  map[string]bool
	failOn   string
	failWith error
}

// gone reports whether the fake's store no longer has the entry.
func (f *teardownFake) gone(id string) bool { return f.missing[id] }

func (f *teardownFake) ports() TeardownPorts {
	return TeardownPorts{
		RetireChild: func(slug, reason string) error {
			if f.failOn == slug {
				return f.failWith
			}
			f.retired = append(f.retired, slug)
			f.reasons = append(f.reasons, reason)
			return nil
		},
		Standing: func() []standing.Order { return f.orders },
		RemoveStanding: func(id string) (bool, error) {
			if f.failOn == id {
				return false, f.failWith
			}
			if f.gone(id) {
				return false, nil
			}
			f.removed = append(f.removed, id)
			return true, nil
		},
		SetStandingEnabled: func(id string, enabled bool) error {
			if f.failOn == id {
				return f.failWith
			}
			f.paused = append(f.paused, id)
			return nil
		},
		Schedules: func() []cadence.Entry { return f.entries },
		RemoveSchedule: func(id string) (bool, error) {
			if f.failOn == id {
				return false, f.failWith
			}
			if f.gone(id) {
				return false, nil
			}
			f.removed = append(f.removed, id)
			return true, nil
		},
		SetScheduleEnabled: func(id string, enabled bool) (bool, error) {
			if f.failOn == id {
				return false, f.failWith
			}
			f.paused = append(f.paused, id)
			return true, nil
		},
	}
}

func TestRetireSubagents(t *testing.T) {
	fake := &teardownFake{
		orders:  []standing.Order{{ID: "o1", Agent: "kid", Enabled: true}},
		entries: []cadence.Entry{{ID: "s1", Agent: "kid", Enabled: true}},
	}
	children := []core.Profile{{Slug: "other"}, {Slug: "kid"}}
	n, slugs, err := RetireSubagents(fake.ports(), "parent", children, true)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if n != 2 || len(slugs) != 2 {
		t.Fatalf("retired = %d, slugs = %v, want both children", n, slugs)
	}
	if slugs[0] != "kid" || slugs[1] != "other" {
		t.Errorf("slugs = %v, want them sorted", slugs)
	}
	if len(fake.retired) != 2 {
		t.Errorf("retired = %v, want both children", fake.retired)
	}
	if fake.reasons[0] != "parent/owner parent removed" {
		t.Errorf("reason = %q", fake.reasons[0])
	}
	// The retired child's wake sources are paused, not removed.
	if len(fake.paused) != 2 {
		t.Errorf("paused = %v, want the standing order and the schedule", fake.paused)
	}
	if len(fake.removed) != 0 {
		t.Errorf("removed = %v, want nothing removed", fake.removed)
	}
}

func TestRetireSubagents_SkipsAlreadyRetiredChildren(t *testing.T) {
	fake := &teardownFake{}
	children := []core.Profile{{Slug: "kid", Retired: true}}
	n, slugs, err := RetireSubagents(fake.ports(), "parent", children, true)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if n != 0 || len(slugs) != 0 {
		t.Errorf("retired = %d, slugs = %v, want none", n, slugs)
	}
	if len(fake.retired) != 0 {
		t.Errorf("retired = %v, want none", fake.retired)
	}
}

func TestRetireSubagents_OffDoesNothing(t *testing.T) {
	fake := &teardownFake{}
	n, slugs, err := RetireSubagents(fake.ports(), "parent", []core.Profile{{Slug: "kid"}}, false)
	if err != nil || n != 0 || len(slugs) != 0 {
		t.Errorf("retire off = %d/%v/%v, want nothing", n, slugs, err)
	}
	if len(fake.retired) != 0 {
		t.Errorf("retired = %v, want none", fake.retired)
	}
}

func TestRetireSubagents_ReportsTheFailure(t *testing.T) {
	fake := &teardownFake{failOn: "kid", failWith: errors.New("kernel down")}
	_, _, err := RetireSubagents(fake.ports(), "parent", []core.Profile{{Slug: "kid"}}, true)
	if err == nil || err.Error() != "kernel down" {
		t.Errorf("err = %v, want the store's error", err)
	}
}

func TestRemoveStandingOrders(t *testing.T) {
	fake := &teardownFake{orders: []standing.Order{
		{ID: "o1", Agent: "ops"},
		{ID: "o2", Agent: "OPS"},
		{ID: "o3", Agent: "other"},
	}}
	n, err := RemoveStandingOrders(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n != 2 {
		t.Errorf("removed = %d, want the agent's two orders", n)
	}
	if len(fake.removed) != 2 {
		t.Errorf("removed = %v, want only the agent's orders", fake.removed)
	}
	for _, id := range fake.removed {
		if id == "o3" {
			t.Errorf("removed = %v, want another agent's order left alone", fake.removed)
		}
	}
}

func TestRemoveStandingOrders_OffDoesNothing(t *testing.T) {
	fake := &teardownFake{orders: []standing.Order{{ID: "o1", Agent: "ops"}}}
	n, err := RemoveStandingOrders(fake.ports(), "ops", false)
	if err != nil || n != 0 || len(fake.removed) != 0 {
		t.Errorf("remove off = %d/%v/%v, want nothing", n, fake.removed, err)
	}
}

// An order the store no longer has is not counted as removed.
func TestRemoveStandingOrders_CountsOnlyWhatExisted(t *testing.T) {
	fake := &teardownFake{
		orders:  []standing.Order{{ID: "o1", Agent: "ops"}, {ID: "o2", Agent: "ops"}},
		missing: map[string]bool{"o2": true},
	}
	n, err := RemoveStandingOrders(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want only the order that existed", n)
	}
}

func TestPauseStandingOrders(t *testing.T) {
	fake := &teardownFake{orders: []standing.Order{
		{ID: "o1", Agent: "ops", Enabled: true},
		{ID: "o2", Agent: "ops", Enabled: false},
		{ID: "o3", Agent: "other", Enabled: true},
	}}
	n, err := PauseStandingOrders(fake.ports(), "ops")
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	if n != 1 {
		t.Errorf("paused = %d, want only the enabled order", n)
	}
	if len(fake.paused) != 1 || fake.paused[0] != "o1" {
		t.Errorf("paused = %v, want o1", fake.paused)
	}
}

func TestCountPausedStandingOrders(t *testing.T) {
	fake := &teardownFake{orders: []standing.Order{
		{ID: "o1", Agent: "ops", Enabled: false},
		{ID: "o2", Agent: "ops", Enabled: true},
		{ID: "o3", Agent: "other", Enabled: false},
	}}
	if n := CountPausedStandingOrders(fake.ports(), "ops"); n != 1 {
		t.Errorf("paused orders = %d, want 1", n)
	}
}

func TestRemoveSchedules(t *testing.T) {
	fake := &teardownFake{entries: []cadence.Entry{
		{ID: "s1", Agent: "ops"},
		{ID: "s2", Agent: "other"},
	}}
	n, err := RemoveSchedules(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want the agent's one schedule", n)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "s1" {
		t.Errorf("removed = %v, want s1", fake.removed)
	}
}

func TestRemoveSchedules_OffDoesNothing(t *testing.T) {
	fake := &teardownFake{entries: []cadence.Entry{{ID: "s1", Agent: "ops"}}}
	n, err := RemoveSchedules(fake.ports(), "ops", false)
	if err != nil || n != 0 || len(fake.removed) != 0 {
		t.Errorf("remove off = %d/%v/%v, want nothing", n, fake.removed, err)
	}
}

// A schedule the store no longer has is not counted as removed.
func TestRemoveSchedules_CountsOnlyWhatExisted(t *testing.T) {
	fake := &teardownFake{
		entries: []cadence.Entry{{ID: "s1", Agent: "ops"}, {ID: "s2", Agent: "ops"}},
		missing: map[string]bool{"s2": true},
	}
	n, err := RemoveSchedules(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want only the schedule that existed", n)
	}
}

func TestPauseSchedules(t *testing.T) {
	fake := &teardownFake{entries: []cadence.Entry{
		{ID: "s1", Agent: "ops", Enabled: true},
		{ID: "s2", Agent: "ops", Enabled: false},
		{ID: "s3", Agent: "other", Enabled: true},
	}}
	n, err := PauseSchedules(fake.ports(), "ops")
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	if n != 1 {
		t.Errorf("paused = %d, want only the enabled schedule", n)
	}
	if len(fake.paused) != 1 || fake.paused[0] != "s1" {
		t.Errorf("paused = %v, want s1", fake.paused)
	}
}

func TestCountPausedSchedules(t *testing.T) {
	fake := &teardownFake{entries: []cadence.Entry{
		{ID: "s1", Agent: "ops", Enabled: false},
		{ID: "s2", Agent: "ops", Enabled: true},
		{ID: "s3", Agent: "other", Enabled: false},
	}}
	if n := CountPausedSchedules(fake.ports(), "ops"); n != 1 {
		t.Errorf("paused schedules = %d, want 1", n)
	}
}

func TestPauseSchedules_ReportsTheFailure(t *testing.T) {
	fake := &teardownFake{
		entries:  []cadence.Entry{{ID: "s1", Agent: "ops", Enabled: true}},
		failOn:   "s1",
		failWith: errors.New("store down"),
	}
	n, err := PauseSchedules(fake.ports(), "ops")
	if err == nil || err.Error() != "store down" {
		t.Errorf("err = %v, want the store's error", err)
	}
	if n != 0 {
		t.Errorf("paused = %d, want none after the failure", n)
	}
}
