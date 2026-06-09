// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func cronOrder(name string) Order {
	return Order{
		Name:       name,
		Triggers:   []Trigger{{Type: TriggerCron, Schedule: "0 8 * * *"}},
		Initiative: Initiative{Mode: InitiativeActOrAsk, MaxTrust: "L2"},
		Plan:       "brief me",
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		o    Order
		ok   bool
	}{
		{"valid-cron", cronOrder("watch"), true},
		{"valid-event", Order{Name: "n", Triggers: []Trigger{{Type: TriggerEvent, Subject: "github.>"}}}, true},
		{"no-name", Order{Triggers: []Trigger{{Type: TriggerCron, Schedule: "x"}}}, false},
		{"no-triggers", Order{Name: "n"}, false},
		{"cron-no-schedule", Order{Name: "n", Triggers: []Trigger{{Type: TriggerCron}}}, false},
		{"event-no-subject", Order{Name: "n", Triggers: []Trigger{{Type: TriggerEvent}}}, false},
		{"unknown-trigger", Order{Name: "n", Triggers: []Trigger{{Type: "weather"}}}, false},
		{"bad-mode", Order{Name: "n", Triggers: []Trigger{{Type: TriggerCron, Schedule: "x"}}, Initiative: Initiative{Mode: "yolo"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.o)
			if (err == nil) != c.ok {
				t.Errorf("Validate ok=%v, want %v (err=%v)", err == nil, c.ok, err)
			}
		})
	}
}

func TestStore_AddListGet(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	o, err := s.Add(cronOrder("portfolio watch"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if o.ID == "" || !o.Enabled || o.CreatedMS == 0 {
		t.Errorf("Add should assign id/enabled/created, got %+v", o)
	}
	got, ok := s.Get(o.ID)
	if !ok || got.Name != "portfolio watch" {
		t.Errorf("Get(%s) = %+v, ok=%v", o.ID, got, ok)
	}
	if n := s.Count(); n != 1 {
		t.Errorf("Count=%d, want 1", n)
	}
	if _, err := s.Add(Order{Name: ""}); err == nil {
		t.Error("Add of an invalid order should error")
	}
}

func TestStore_PauseResumeRemove(t *testing.T) {
	s, _ := Open(t.TempDir())
	o, _ := s.Add(cronOrder("watch"))

	paused, err := s.SetEnabled(o.ID, false)
	if err != nil || paused.Enabled {
		t.Errorf("pause: enabled=%v err=%v, want disabled", paused.Enabled, err)
	}
	resumed, err := s.SetEnabled(o.ID, true)
	if err != nil || !resumed.Enabled {
		t.Errorf("resume: enabled=%v err=%v, want enabled", resumed.Enabled, err)
	}
	if _, err := s.SetEnabled("nope", false); err != ErrNotFound {
		t.Errorf("SetEnabled unknown = %v, want ErrNotFound", err)
	}
	removed, err := s.Remove(o.ID)
	if err != nil || !removed {
		t.Errorf("Remove = %v err=%v, want true", removed, err)
	}
	if _, ok := s.Get(o.ID); ok {
		t.Error("order should be gone after Remove")
	}
}

func TestStore_Update(t *testing.T) {
	s, _ := Open(t.TempDir())
	o, _ := s.Add(cronOrder("watch"))
	createdMS := o.CreatedMS

	// Edit the mutable fields; the mutator also tries to tamper with id/enabled/
	// created, which the store must protect.
	updated, err := s.Update(o.ID, func(o *Order) {
		o.Name = "renamed watch"
		o.Plan = "do the new thing"
		o.Initiative.Mode = InitiativeAsk
		o.Assure = 3
		o.ID = "hacked"
		o.Enabled = false
		o.CreatedMS = 0
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed watch" || updated.Plan != "do the new thing" {
		t.Errorf("edited fields not applied: %+v", updated)
	}
	if updated.Initiative.Mode != InitiativeAsk || updated.Assure != 3 {
		t.Errorf("mode/assure not applied: %+v", updated)
	}
	// Protected fields survive the tampering.
	if updated.ID != o.ID || !updated.Enabled || updated.CreatedMS != createdMS {
		t.Errorf("identity/lifecycle not protected: %+v (orig id=%s created=%d)", updated, o.ID, createdMS)
	}
	if updated.UpdatedMS < createdMS {
		t.Errorf("UpdatedMS not bumped: %d < %d", updated.UpdatedMS, createdMS)
	}

	// Persisted: reopening the store sees the edit.
	s2, _ := Open(s.path[:len(s.path)-len("/standing.json")])
	got, ok := s2.Get(o.ID)
	if !ok || got.Name != "renamed watch" || got.Assure != 3 {
		t.Errorf("edit not persisted: %+v ok=%v", got, ok)
	}

	// An edit that makes the order invalid is rejected and rolled back.
	if _, err := s.Update(o.ID, func(o *Order) { o.Name = "" }); err == nil {
		t.Error("Update to an invalid order should error")
	}
	if back, _ := s.Get(o.ID); back.Name != "renamed watch" {
		t.Errorf("invalid edit should roll back, name=%q", back.Name)
	}

	// Unknown id → ErrNotFound.
	if _, err := s.Update("nope", func(*Order) {}); err != ErrNotFound {
		t.Errorf("Update unknown = %v, want ErrNotFound", err)
	}
}

// TestStore_PreservesInitiative: the initiative ceiling (mode, max_trust, budget)
// round-trips through Add → Get → reopen, so the budget cap M404 enforces is
// actually persisted from what the operator set.
func TestStore_PreservesInitiative(t *testing.T) {
	dir := t.TempDir()
	s1, _ := Open(dir)
	o, err := s1.Add(Order{
		Name:       "capped",
		Triggers:   []Trigger{{Type: TriggerCron, Schedule: "0 8 * * *"}},
		Initiative: Initiative{Mode: InitiativeActOrAsk, MaxTrust: "L2", BudgetPerRunMc: 1_000_000_000},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	s2, _ := Open(dir)
	got, ok := s2.Get(o.ID)
	if !ok {
		t.Fatal("order did not persist")
	}
	if got.Initiative.BudgetPerRunMc != 1_000_000_000 {
		t.Errorf("budget = %d, want 1e9 microcents ($1)", got.Initiative.BudgetPerRunMc)
	}
	if got.Initiative.MaxTrust != "L2" || got.Initiative.Mode != InitiativeActOrAsk {
		t.Errorf("initiative mode/trust not preserved: %+v", got.Initiative)
	}
}

// TestPruneToLive: the runner/cron per-order bookkeeping maps must not grow
// forever — an entry for an order that no longer exists is dropped, live entries
// are kept intact, and there's no work when nothing is stale (M414).
func TestPruneToLive(t *testing.T) {
	m := map[string]int64{"a": 1, "b": 2, "gone": 3}
	pruneToLive(m, []Order{{ID: "a"}, {ID: "b"}})
	if _, ok := m["gone"]; ok {
		t.Error("a removed order's entry should be pruned")
	}
	if len(m) != 2 || m["a"] != 1 || m["b"] != 2 {
		t.Errorf("live entries must be kept intact, got %v", m)
	}
	// No stale entries (len(map) <= len(orders)) → untouched.
	m2 := map[string]int64{"a": 1}
	pruneToLive(m2, []Order{{ID: "a"}, {ID: "b"}})
	if len(m2) != 1 || m2["a"] != 1 {
		t.Errorf("no stale entries → map unchanged, got %v", m2)
	}
}

// TestSafeFire_ContainsPanic: a panic while running a fired order must be
// contained, not propagated — the runner and cron loop dispatch every order on its
// own `go fire(...)` goroutine, where an uncovered panic would crash the whole
// daemon (M413, the HIGH finding). safeFire is the backstop that makes the
// package's documented no-crash guarantee true for ANY FireFunc.
func TestSafeFire_ContainsPanic(t *testing.T) {
	ran := false
	panicky := func(_ context.Context, _ Order, _ string) {
		ran = true
		panic("boom from a fired order's plan")
	}
	// Called synchronously: if safeFire did not recover, this line would panic the
	// test goroutine and fail the test.
	safeFire(panicky)(context.Background(), Order{ID: "x", Name: "n"}, "subj")
	if !ran {
		t.Fatal("safeFire should still invoke the wrapped fire")
	}
	// A non-panicking fire runs normally through the wrapper.
	ok := false
	safeFire(func(_ context.Context, _ Order, _ string) { ok = true })(context.Background(), Order{}, "")
	if !ok {
		t.Error("safeFire should pass through a normal fire")
	}
}

// TestStore_RollsBackOnSaveFailure: when the durable write fails, SetEnabled and
// Remove must leave the in-memory view byte-identical to disk — never a state that
// diverges from what survives a reopen (M412, BUG 5 fix). The failure is forced by
// turning the atomic-write temp path into a directory so os.WriteFile errors.
func TestStore_RollsBackOnSaveFailure(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	o, _ := s.Add(cronOrder("watch"))

	// Make save() fail: the temp path WriteFile targets is now a directory.
	tmp := filepath.Join(dir, "standing.json.tmp")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}

	// SetEnabled must report the error AND not mutate the live order.
	if _, err := s.SetEnabled(o.ID, false); err == nil {
		t.Fatal("SetEnabled should error when the durable write fails")
	}
	if got, _ := s.Get(o.ID); !got.Enabled {
		t.Error("SetEnabled must roll back Enabled when save fails")
	}

	// Remove must report the error AND keep the order present.
	if removed, err := s.Remove(o.ID); err == nil || removed {
		t.Errorf("Remove should error and not remove when save fails (removed=%v err=%v)", removed, err)
	}
	if _, ok := s.Get(o.ID); !ok {
		t.Error("Remove must roll back (keep the order) when save fails")
	}
	if n := s.Count(); n != 1 {
		t.Errorf("Count=%d after failed mutations, want 1 (no divergence)", n)
	}
}

// TestStore_Persists: orders survive a reopen (durable JSON file).
func TestStore_Persists(t *testing.T) {
	dir := t.TempDir()
	s1, _ := Open(dir)
	o, _ := s1.Add(cronOrder("durable"))
	_, _ = s1.SetEnabled(o.ID, false)

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := s2.Get(o.ID)
	if !ok {
		t.Fatal("order did not persist across reopen")
	}
	if got.Enabled {
		t.Error("paused state did not persist")
	}
}
