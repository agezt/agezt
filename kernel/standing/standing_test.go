// SPDX-License-Identifier: MIT

package standing

import (
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
