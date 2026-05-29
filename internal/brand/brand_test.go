// SPDX-License-Identifier: MIT

package brand

import "testing"

func TestFrozenIdentity(t *testing.T) {
	// DECISIONS A1 freezes these values. Changing them is a deliberate
	// rename and must update this test plus every release artifact.
	cases := map[string]string{
		"Name":      Name,
		"Binary":    Binary,
		"CLI":       CLI,
		"EnvPrefix": EnvPrefix,
		"ConfigDir": ConfigDir,
	}
	want := map[string]string{
		"Name":      "Agezt",
		"Binary":    "agezt",
		"CLI":       "agt",
		"EnvPrefix": "AGEZT_",
		"ConfigDir": ".agezt",
	}
	for k, got := range cases {
		if got != want[k] {
			t.Errorf("brand.%s = %q, want %q", k, got, want[k])
		}
	}
	if ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1 (DECISIONS B1)", ProtocolVersion)
	}
	if Version == "" {
		t.Error("Version must be set")
	}
}
