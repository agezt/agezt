// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsWritesNativeSelectedRootPersistencePinnedAndRegistry(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "")
	t.Setenv(creds.AutoEncryptEnvVar, "off")
	t.Setenv("AGEZT_W45B_NATIVE_PUBLIC", "")
	t.Setenv("AGEZT_W45B_NATIVE_SECRET", "")
	root := t.TempDir()
	kernelDir, serverDir := filepath.Join(root, "kernel"), filepath.Join(root, "server")
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: kernelDir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, serverDir)
	s.configEnvPinned = map[string]bool{"AGEZT_W45B_NATIVE_PUBLIC": true}
	svc := s.settingsWrites()
	sec := settings.Section{ID: "owned-native-writes", Name: "Owned", Locked: true, Fields: []settings.Field{{Env: "AGEZT_W45B_NATIVE_PUBLIC", Type: settings.TypeText, Apply: settings.ApplyLive}, {Env: "AGEZT_W45B_NATIVE_SECRET", Type: settings.TypePassword, Secret: true}}}
	head, hash := k.Journal().Head()
	out, err := svc.Register(context.Background(), appsettings.RegisterInput{Section: sec})
	if err != nil || out.ID != sec.ID || out.Applied != "restart" {
		t.Fatal(out, err)
	}
	field, ok := settings.NewRegistry(serverDir).FieldByEnv("AGEZT_W45B_NATIVE_PUBLIC")
	if !ok || field.Apply != settings.ApplyRestart {
		t.Fatal("selected registered schema missing/restart normalization lost")
	}
	if _, ok := settings.NewRegistry(kernelDir).FieldByEnv("AGEZT_W45B_NATIVE_PUBLIC"); ok {
		t.Fatal("registry wrote kernel directory")
	}
	setOut, err := svc.Set(context.Background(), appsettings.SetInput{Name: " AGEZT_W45B_NATIVE_PUBLIC ", Value: " raw "})
	if err != nil || setOut.EnvPinned != true || setOut.Applied != "restart" || os.Getenv("AGEZT_W45B_NATIVE_PUBLIC") != "" {
		t.Fatal(setOut, err)
	}
	store := settings.NewStore(serverDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if value, _ := store.Get("AGEZT_W45B_NATIVE_PUBLIC"); value != "raw" {
		t.Fatal("selected config was not persisted")
	}
	if _, err := os.Stat(settings.NewStore(kernelDir).Path); !os.IsNotExist(err) {
		t.Fatal("config wrote kernel root", err)
	}
	setOut, err = svc.Set(context.Background(), appsettings.SetInput{Name: "AGEZT_W45B_NATIVE_SECRET", Value: " owned-marker "})
	if err != nil || setOut.Saved != true {
		t.Fatal(setOut, err)
	}
	vault := creds.NewStore(serverDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if !vault.Has("AGEZT_W45B_NATIVE_SECRET") || vault.Get("AGEZT_W45B_NATIVE_SECRET") != "owned-marker" {
		t.Fatal("selected vault missing value")
	}
	if _, err := svc.Set(context.Background(), appsettings.SetInput{Name: "AGEZT_W45B_NATIVE_SECRET", Value: "  "}); err != nil {
		t.Fatal(err)
	}
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if vault.Has("AGEZT_W45B_NATIVE_SECRET") {
		t.Fatal("secret clear not persisted")
	}
	if _, err := svc.Unregister(context.Background(), appsettings.UnregisterInput{ID: sec.ID}); err == nil {
		t.Fatal("locked section removed without force")
	}
	unregisterOut, err := svc.Unregister(context.Background(), appsettings.UnregisterInput{ID: " " + sec.ID + " ", Force: true})
	if err != nil || unregisterOut.Removed != true {
		t.Fatal(unregisterOut, err)
	}
	unregisterOut, err = svc.Unregister(context.Background(), appsettings.UnregisterInput{ID: sec.ID, Force: true})
	if err != nil || unregisterOut.Removed != false {
		t.Fatal(unregisterOut, err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("direct service unexpectedly audits/calls provider")
	}
}
