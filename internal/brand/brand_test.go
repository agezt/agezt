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

// TestBuildStampReturnsPackageVars is the companion to the Makefile /
// scripts/build.sh ldflag injection: BuildStamp MUST return the current
// value of Version / BuildCommit / BuildTime, which is what `-X` patches.
// If a future refactor accidentally returns cached literals here, the
// stamped binary will report the wrong identity and this test catches it.
func TestBuildStampReturnsPackageVars(t *testing.T) {
	origVer, origCommit, origTime := Version, BuildCommit, BuildTime
	t.Cleanup(func() {
		Version, BuildCommit, BuildTime = origVer, origCommit, origTime
	})

	Version = "9.9.9-test"
	BuildCommit = "abcdef0"
	BuildTime = "2099-01-01T00:00:00Z"

	gotVer, gotCommit, gotTime := BuildStamp()
	if gotVer != "9.9.9-test" || gotCommit != "abcdef0" || gotTime != "2099-01-01T00:00:00Z" {
		t.Errorf("BuildStamp() = (%q, %q, %q), want the same triple as the package vars", gotVer, gotCommit, gotTime)
	}

	// And the empty-default case (no `-X` ldflag stamps applied).
	Version = "1.0.0" // the package default; tests must not mutate Version persistently.
	BuildCommit = ""
	BuildTime = ""
	ver, commit, btime := BuildStamp()
	if ver != "1.0.0" {
		t.Errorf("BuildStamp().version = %q, want %q (default)", ver, "1.0.0")
	}
	if commit != "" || btime != "" {
		t.Errorf("BuildStamp() = (%q, %q) in unstamped build — both should be empty", commit, btime)
	}
}
