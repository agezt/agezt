// SPDX-License-Identifier: MIT

package settings

import (
	"sync"
	"testing"
)

func loaded(t *testing.T, dir string) *Store {
	t.Helper()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestSave_ConcurrentWritersKeepEachOthersSettings: every control-plane handler
// opens its own Store (Load → Set → Save), as do `agt config` and the daemon.
// A Store saving its whole stale map used to revert whatever another writer
// saved after it loaded.
func TestSave_ConcurrentWritersKeepEachOthersSettings(t *testing.T) {
	dir := t.TempDir()
	seed := loaded(t, dir)
	seed.Set("AGEZT_KEEP", "1")
	seed.Set("AGEZT_DROP", "1")
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}
	routing := loaded(t, dir)
	persona := loaded(t, dir)

	routing.Set("AGEZT_TASK_MODEL_CHAINS", "plan=a,b")
	if err := routing.Save(); err != nil {
		t.Fatal(err)
	}
	persona.Set("AGEZT_SYSTEM_PROMPT", "be brief")
	persona.Remove("AGEZT_DROP")
	if err := persona.Save(); err != nil {
		t.Fatal(err)
	}

	final := loaded(t, dir).All()
	want := map[string]string{"AGEZT_KEEP": "1", "AGEZT_TASK_MODEL_CHAINS": "plan=a,b", "AGEZT_SYSTEM_PROMPT": "be brief"}
	if len(final) != len(want) {
		t.Fatalf("settings = %v, want %v", final, want)
	}
	for k, v := range want {
		if final[k] != v {
			t.Fatalf("settings = %v, want %v", final, want)
		}
	}
}

// TestSave_ParallelHandlers: concurrent handlers each saving their own key all land.
func TestSave_ParallelHandlers(t *testing.T) {
	dir := t.TempDir()
	keys := []string{"AGEZT_A", "AGEZT_B", "AGEZT_C", "AGEZT_D", "AGEZT_E", "AGEZT_F"}
	var wg sync.WaitGroup
	for _, k := range keys {
		s := loaded(t, dir)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Set(k, "v")
			if err := s.Save(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := loaded(t, dir).Names(); len(got) != len(keys) {
		t.Fatalf("names = %v, want all %d", got, len(keys))
	}
}
