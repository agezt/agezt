// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsReadsSelectedServerRootFreshnessLegacyLoadErrorsAndPrivacy(t *testing.T) {
	t.Setenv("AGEZT_W45_NATIVE_PUBLIC", "")
	t.Setenv("AGEZT_W45_NATIVE_SECRET", "never-read-env-value")
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "")
	root := t.TempDir()
	kernelDir, serverDir := filepath.Join(root, "kernel"), filepath.Join(root, "server")
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: kernelDir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	reg := settings.NewRegistry(serverDir)
	sec := settings.Section{ID: "owned-native-read", Name: "selected", Fields: []settings.Field{{Env: "AGEZT_W45_NATIVE_PUBLIC", Type: settings.TypeText}, {Env: "AGEZT_W45_NATIVE_SECRET", Type: settings.TypePassword, Secret: true}}}
	if err := reg.Register(sec); err != nil {
		t.Fatal(err)
	}
	wrong := settings.NewStore(kernelDir)
	wrong.Set("AGEZT_W45_NATIVE_PUBLIC", "wrong-root")
	if err := wrong.Save(); err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(serverDir)
	store.Set("AGEZT_W45_NATIVE_PUBLIC", " selected raw ")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	vaultPath := creds.NewStore(serverDir).Path
	if err := os.WriteFile(vaultPath, []byte(`{"AGEZT_W45_NATIVE_SECRET":"owned-private-marker"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, serverDir)
	s.configEnvPinned = map[string]bool{"AGEZT_W45_NATIVE_PUBLIC": true}
	svc := s.settingsReads()
	head, hash := k.Journal().Head()
	schema, err := svc.Schema(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, section := range schema.Sections {
		if section.ID == sec.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("schema read kernel root instead of server root")
	}
	read := func() map[string]map[string]any {
		out, err := svc.Values(context.Background(), struct{}{})
		if err != nil {
			t.Fatal(err)
		}
		rows := map[string]map[string]any{}
		for _, row := range out.Fields {
			rows[row.Env] = map[string]any{"env": row.Env, "secret": row.Secret, "env_pinned": row.EnvPinned, "set": row.Set}
			if row.Value != nil {
				rows[row.Env]["value"] = *row.Value
			}
		}
		return rows
	}
	rows := read()
	if rows["AGEZT_W45_NATIVE_PUBLIC"]["value"] != " selected raw " || rows["AGEZT_W45_NATIVE_PUBLIC"]["env_pinned"] != true || rows["AGEZT_W45_NATIVE_SECRET"]["set"] != true {
		t.Fatal("selected values/pins/presence differ")
	}
	if _, ok := rows["AGEZT_W45_NATIVE_SECRET"]["value"]; ok {
		t.Fatal("secret value exposed")
	}
	store.Set("AGEZT_W45_NATIVE_PUBLIC", "fresh-store")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	if read()["AGEZT_W45_NATIVE_PUBLIC"]["value"] != "fresh-store" {
		t.Fatal("stale store snapshot")
	}
	t.Setenv("AGEZT_W45_NATIVE_PUBLIC", " raw live ")
	if read()["AGEZT_W45_NATIVE_PUBLIC"]["value"] != " raw live " {
		t.Fatal("store overrode live env")
	}
	t.Setenv("AGEZT_W45_NATIVE_PUBLIC", "")
	for _, path := range []string{store.Path, vaultPath} {
		if err := os.WriteFile(path, []byte(`{broken`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows = read()
	if rows["AGEZT_W45_NATIVE_PUBLIC"]["value"] != "" || rows["AGEZT_W45_NATIVE_PUBLIC"]["set"] != false || rows["AGEZT_W45_NATIVE_SECRET"]["set"] != false {
		t.Fatal("legacy ignored load errors changed")
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("read used journal/provider")
	}
}
