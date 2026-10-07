// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelAccountsRegisteredFieldsNativeLifecycle(t *testing.T) {
	builtinchannels.RegisterAll()
	original, ok := channel.LookupManifest("email")
	if !ok {
		t.Fatal("missing builtin fixture")
	}
	defer channel.RegisterManifest(original)
	manifest := original
	manifest.ConfigSection = "owned-w47c-account"
	manifest.RequiredEnv = []string{"AGEZT_W47C_PUBLIC", "AGEZT_W47C_SECRET"}
	channel.RegisterManifest(manifest)
	t.Setenv(creds.PassphraseEnvVar, "")
	t.Setenv(creds.AutoEncryptEnvVar, "off")
	for _, name := range []string{"AGEZT_W47C_PUBLIC", "AGEZT_W47C_SECRET"} {
		for _, label := range []string{"", "work", "other"} {
			t.Setenv(settings.SuffixEnv(name, label), "")
		}
	}
	p := mock.New()
	k, _, client, dir := startPair(t, p)
	ctx := context.Background()
	reg := settings.NewRegistry(dir)
	if err := reg.Register(settings.Section{ID: manifest.ConfigSection, Name: "Owned", Fields: []settings.Field{{Env: "AGEZT_W47C_PUBLIC", Type: settings.TypeText}, {Env: "AGEZT_W47C_SECRET", Type: settings.TypePassword, Secret: true}}}); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"", "work", "other"} {
		for _, name := range []string{"AGEZT_W47C_PUBLIC", "AGEZT_W47C_SECRET"} {
			if _, err := client.Call(ctx, controlplane.CmdChannelAccountSet, map[string]any{"kind": "email", "label": label, "name": name, "value": " owned value "}); err != nil {
				t.Fatal("registered set", err)
			}
		}
	}
	before, err := client.Call(ctx, controlplane.CmdChannelList, nil)
	if err != nil || !hasAccount(before, "email", "work") {
		t.Fatal("registered account not listed", err)
	}
	result, err := client.Call(ctx, controlplane.CmdChannelAccountRemove, map[string]any{"kind": "email", "label": "work"})
	if err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	vault := creds.NewStore(dir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	_, publicRemains := store.Get("AGEZT_W47C_PUBLIC#work")
	secretRemains := vault.Has("AGEZT_W47C_SECRET#work")
	after, err := client.Call(ctx, controlplane.CmdChannelList, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed := hasAccount(after, "email", "work")
	if result["removed"] != float64(2) || publicRemains || secretRemains || listed {
		t.Errorf("EXPECTED:removed=2, no registered work config/vault keys or listed account ACTUAL:removed=%v public=%v secret=%v listed=%v", result["removed"], publicRemains, secretRemains, listed)
	}
	// Reload independent store objects to prove default/other account persistence.
	for _, label := range []string{"", "other"} {
		key := settings.SuffixEnv("AGEZT_W47C_PUBLIC", label)
		if value, ok := store.Get(key); !ok || value != "owned value" || !vault.Has(settings.SuffixEnv("AGEZT_W47C_SECRET", label)) {
			t.Fatal("other/default account changed", label)
		}
	}
	if p.CallCount() != 0 {
		t.Fatal("account lifecycle called provider")
	}
	_ = k
}
