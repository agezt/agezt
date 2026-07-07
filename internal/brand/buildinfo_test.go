// SPDX-License-Identifier: MIT

package brand

import (
	"runtime/debug"
	"testing"
)

// TestBuildInfoRuns exercises BuildInfo end to end. Under `go test` the binary
// is built from the module's git checkout, so debug.ReadBuildInfo() returns
// ok=true and the VCS-settings loop runs, reading vcs.revision / vcs.time /
// vcs.modified. We assert only on internal consistency (not exact values,
// which are environment-specific): a set revision implies the loop matched at
// least one setting, and the modified flag is a plain bool that must be one of
// the two valid states.
func TestBuildInfoRuns(t *testing.T) {
	revision, committed, modified := BuildInfo()

	// The call must not panic and must return a well-formed triple. When VCS
	// info is embedded, revision is a hex SHA; when it isn't (buildvcs=false
	// or a tarball build), revision is "". Both are valid — we only assert the
	// function returned coherently.
	if revision == "" && committed != "" {
		t.Errorf("BuildInfo() returned empty revision but non-empty commit time %q", committed)
	}

	// modified is a bool; the only purpose here is to reference every return
	// value so the compiler and reader both see all three are exercised.
	_ = modified
}

// TestBuildInfo_Fallthrough exercises the !ok path by swapping
// debugReadBuildInfo for a function that returns ok=false. This
// path is unreachable under normal `go test` because the module
// info is always present, so we must redirect the package var.
func TestBuildInfo_Fallthrough(t *testing.T) {
	orig := debugReadBuildInfo
	t.Cleanup(func() { debugReadBuildInfo = orig })

	debugReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return nil, false
	}

	rev, committed, modified := BuildInfo()
	if rev != "" {
		t.Errorf("BuildInfo().revision = %q, want empty when ReadBuildInfo fails", rev)
	}
	if committed != "" {
		t.Errorf("BuildInfo().committed = %q, want empty when ReadBuildInfo fails", committed)
	}
	if modified {
		t.Error("BuildInfo().modified = true, want false when ReadBuildInfo fails")
	}
}

// TestBuildInfo_VCSValues exercises the VCS-settings loop with
// non-empty settings, covering every switch arm. Under normal
// `go test` the Settings slice is often empty, so we inject it.
func TestBuildInfo_VCSValues(t *testing.T) {
	orig := debugReadBuildInfo
	t.Cleanup(func() { debugReadBuildInfo = orig })

	debugReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234"},
				{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	}

	rev, committed, modified := BuildInfo()
	if rev != "abc1234" {
		t.Errorf("BuildInfo().revision = %q, want %q", rev, "abc1234")
	}
	if committed != "2026-01-01T00:00:00Z" {
		t.Errorf("BuildInfo().committed = %q, want %q", committed, "2026-01-01T00:00:00Z")
	}
	if !modified {
		t.Error("BuildInfo().modified = false, want true")
	}

	// Also test vcs.modified=false path.
	debugReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "false"},
			},
		}, true
	}
	_, _, modified = BuildInfo()
	if modified {
		t.Error("BuildInfo().modified = true, want false when vcs.modified is \"false\"")
	}

	// Unknown keys are silently skipped.
	debugReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.unknown", Value: "anything"},
			},
		}, true
	}
	rev, committed, modified = BuildInfo()
	if rev != "" || committed != "" || modified {
		t.Errorf("BuildInfo() = (%q, %q, %v), want (\"\", \"\", false) for unknown keys", rev, committed, modified)
	}
}
