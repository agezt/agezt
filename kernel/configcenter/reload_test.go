// SPDX-License-Identifier: MIT

package configcenter

import "testing"

// TestEntriesSurviveARestart: the loader compared the first seven bytes of each
// file name with the six-byte "entry_", so it never loaded anything — every
// config-center entry vanished at each daemon restart, since the package
// shipped.
func TestEntriesSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	c, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set(&ConfigEntry{Key: "feature.flag", Value: "on", Rating: RatingPublic}); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	e, err := reopened.GetEntry("feature.flag")
	if err != nil || e.Value != "on" {
		t.Fatalf("entry after restart = %+v, err %v; want it reloaded", e, err)
	}
}
