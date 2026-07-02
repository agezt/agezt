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

// TestChannelAccountSetRemove drives the multi-account API end-to-end: a labelled
// email account's non-secret field lands in the config store at the "#label"
// suffixed key, its secret lands in the vault, the channel list reflects the new
// account, and remove deletes both.
func TestChannelAccountSetRemove(t *testing.T) {
	builtinchannels.RegisterAll() // make the email manifest + section available
	_, _, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Non-secret field → config store at AGEZT_EMAIL_SMTP_ADDR#work.
	if _, err := c.Call(ctx, controlplane.CmdChannelAccountSet, map[string]any{
		"kind": "email", "label": "work", "name": "AGEZT_EMAIL_SMTP_ADDR", "value": "smtp.work.test:587",
	}); err != nil {
		t.Fatalf("set smtp: %v", err)
	}
	// Secret field → vault at AGEZT_EMAIL_PASSWORD#work.
	if _, err := c.Call(ctx, controlplane.CmdChannelAccountSet, map[string]any{
		"kind": "email", "label": "work", "name": "AGEZT_EMAIL_PASSWORD", "value": "s3cr3t",
	}); err != nil {
		t.Fatalf("set password: %v", err)
	}

	store := settings.NewStore(dir)
	_ = store.Load()
	if v, ok := store.Get("AGEZT_EMAIL_SMTP_ADDR#work"); !ok || v != "smtp.work.test:587" {
		t.Fatalf("store missing suffixed non-secret: %q ok=%v", v, ok)
	}
	if _, ok := store.Get("AGEZT_EMAIL_PASSWORD#work"); ok {
		t.Fatal("secret must NOT be in the config store")
	}
	vault := creds.NewStore(dir)
	_ = vault.Load()
	if !vault.Has("AGEZT_EMAIL_PASSWORD#work") {
		t.Fatal("vault missing suffixed secret")
	}

	// Bad label rejected.
	if _, err := c.Call(ctx, controlplane.CmdChannelAccountSet, map[string]any{
		"kind": "email", "label": "Bad Label", "name": "AGEZT_EMAIL_FROM", "value": "x@y.z",
	}); err == nil {
		t.Fatal("invalid label should be rejected")
	}
	// A field that isn't part of the channel's section is rejected.
	if _, err := c.Call(ctx, controlplane.CmdChannelAccountSet, map[string]any{
		"kind": "email", "label": "work", "name": "AGEZT_TELEGRAM_TOKEN", "value": "x",
	}); err == nil {
		t.Fatal("cross-section field should be rejected")
	}

	// The channel list surfaces the new account.
	res, err := c.Call(ctx, controlplane.CmdChannelList, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAccount(res, "email", "work") {
		t.Fatal("channel list should include the 'work' email account")
	}

	// Remove deletes the account's stored fields across store + vault.
	if _, err := c.Call(ctx, controlplane.CmdChannelAccountRemove, map[string]any{
		"kind": "email", "label": "work",
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	store2 := settings.NewStore(dir)
	_ = store2.Load()
	if _, ok := store2.Get("AGEZT_EMAIL_SMTP_ADDR#work"); ok {
		t.Fatal("store key should be gone after remove")
	}
	vault2 := creds.NewStore(dir)
	_ = vault2.Load()
	if vault2.Has("AGEZT_EMAIL_PASSWORD#work") {
		t.Fatal("vault key should be gone after remove")
	}
}

func TestChannelListIncludesProbeMatrix(t *testing.T) {
	builtinchannels.RegisterAll()
	t.Setenv("AGEZT_TELEGRAM_TOKEN", "tok-test")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	channel.SetLive([]string{"telegram"})
	channel.SetLiveInstances([]string{"telegram"})
	t.Cleanup(func() {
		channel.SetLive(nil)
		channel.SetLiveInstances(nil)
	})

	res, err := c.Call(context.Background(), controlplane.CmdChannelList, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res["probe_matrix"].(map[string]any); !ok {
		t.Fatalf("channel list missing probe_matrix: %v", res)
	}
	if _, ok := res["media_matrix"].(map[string]any); !ok {
		t.Fatalf("channel list missing media_matrix: %v", res)
	}
	row := findChannel(res, "telegram")
	if row == nil {
		t.Fatal("telegram row missing")
	}
	probe, _ := row["probe"].(map[string]any)
	if probe["roundtrip_status"] != "ready" || probe["mode"] != "two_way" {
		t.Fatalf("telegram probe = %v, want ready two_way", probe)
	}
	accounts, _ := row["accounts"].([]any)
	if len(accounts) == 0 {
		t.Fatal("telegram accounts missing")
	}
	account, _ := accounts[0].(map[string]any)
	accountProbe, _ := account["probe"].(map[string]any)
	if accountProbe["roundtrip_ready"] != true {
		t.Fatalf("telegram account probe = %v, want roundtrip_ready", accountProbe)
	}
}

func hasAccount(res map[string]any, kind, label string) bool {
	row := findChannel(res, kind)
	if row == nil {
		return false
	}
	accts, _ := row["accounts"].([]any)
	for _, a := range accts {
		ar, _ := a.(map[string]any)
		if ar["label"] == label {
			return true
		}
	}
	return false
}

func findChannel(res map[string]any, kind string) map[string]any {
	chans, _ := res["channels"].([]any)
	for _, c := range chans {
		row, _ := c.(map[string]any)
		if row["kind"] != kind {
			continue
		}
		return row
	}
	return nil
}
