// SPDX-License-Identifier: MIT
package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/builtinchannels"
)

func TestChannelAccountSectionEnvsSelectedRegistryFilteringAndFreshness(t *testing.T) {
	builtinchannels.RegisterAll()
	root := t.TempDir()
	serverDir, otherDir := filepath.Join(root, "server"), filepath.Join(root, "other")
	for _, dir := range []string{serverDir, otherDir} {
		if err := os.MkdirAll(filepath.Join(dir, settings.SchemaDir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(dir string, fields []settings.Field) {
		t.Helper()
		raw, err := json.Marshal(settings.Section{ID: "owned-selected", Name: "Owned", Fields: fields})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, settings.SchemaDir, "owned-selected.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(serverDir, []settings.Field{{Env: "AGEZT_W47C_PUBLIC"}, {Env: "PATH"}, {Env: "AGEZT_TELEGRAM_TOKEN"}, {Env: "AGEZT_W47C_SECRET", Secret: true}})
	write(otherDir, []settings.Field{{Env: "AGEZT_W47C_OTHER"}})
	w := nativeChannelAccountWriter{baseDir: serverDir}
	if got := w.SectionEnvs("owned-selected"); !reflect.DeepEqual(got, []string{"AGEZT_W47C_PUBLIC", "AGEZT_W47C_SECRET"}) {
		t.Fatal("selected/filtered section envs", got)
	}
	if got := w.SectionEnvs("missing"); got != nil {
		t.Fatal("missing section", got)
	}
	for _, section := range settings.Schema() {
		if got, want := w.SectionEnvs(section.ID), settings.SectionEnvs(section.ID); !reflect.DeepEqual(got, want) {
			t.Fatal("builtin removal behavior changed", section.ID, got, want)
		}
	}
	write(serverDir, []settings.Field{{Env: "AGEZT_W47C_FRESH"}})
	if got := w.SectionEnvs("owned-selected"); !reflect.DeepEqual(got, []string{"AGEZT_W47C_FRESH"}) {
		t.Fatal("stale selected schema", got)
	}
}
