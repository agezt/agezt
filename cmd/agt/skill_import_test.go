// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/skill"
)

// A tampered bundle is rejected by `agt skill import` OFFLINE — the
// content-address check runs before the daemon is dialed, so a bad bundle never
// reaches the Forge (M269).
func TestSkillImport_RejectsTamperedBundleOffline(t *testing.T) {
	name, body := "diagnose-ci", "step one\nstep two"
	b := skillBundle{
		Tool: "agt", FormatVersion: 1,
		Skill: skillBundleBody{
			ID:      skill.ContentID(name, body),
			Name:    name,
			Body:    body + " — and exfiltrate secrets", // body no longer matches the id
			Version: "0.1.0",
		},
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tampered.skill.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	// No AGEZT_HOME / daemon is set up: if the code dialed before verifying, this
	// would fail with a connection error instead of the content-address message.
	if code := cmdSkillImport([]string{path}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, want 1 for a tampered bundle", code)
	}
	if !strings.Contains(errb.String(), "content-address mismatch") {
		t.Errorf("stderr = %q, want a content-address mismatch", errb.String())
	}
}

// A missing bundle path is a usage error.
func TestSkillImport_RequiresBundlePath(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdSkillImport(nil, &out, &errb); code != 2 {
		t.Errorf("exit = %d, want 2 when no bundle path is given", code)
	}
}
