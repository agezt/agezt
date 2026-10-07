// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelAccountsNativeSelectedServerRootPersistenceAndNoDirectAudit(t *testing.T) {
	builtinchannels.RegisterAll()
	t.Setenv(creds.PassphraseEnvVar, "")
	t.Setenv(creds.AutoEncryptEnvVar, "off")
	t.Setenv("AGEZT_EMAIL_SMTP_ADDR#work", "")
	t.Setenv("AGEZT_EMAIL_PASSWORD#work", "")
	root := t.TempDir()
	kernelDir, serverDir := filepath.Join(root, "kernel"), filepath.Join(root, "server")
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: kernelDir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, serverDir)
	svc := s.channelAccounts()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	head, hash := k.Journal().Head()
	out, err := svc.Set(ctx, appchannels.SetAccountInput{Kind: " EMAIL ", Label: " work ", Name: " AGEZT_EMAIL_SMTP_ADDR ", Value: " smtp.owned.test:587 "})
	if err != nil || !reflect.DeepEqual(out, appchannels.SetAccountOutput{Kind: "email", Label: "work", Env: "AGEZT_EMAIL_SMTP_ADDR#work", Saved: true, Applied: "restart"}) {
		t.Fatal(out, err)
	}
	out, err = svc.Set(ctx, appchannels.SetAccountInput{Kind: "email", Label: "work", Name: "AGEZT_EMAIL_PASSWORD", Value: " owned-sensitive-middle "})
	if err != nil || out.Applied != "restart" {
		t.Fatal(out, err)
	}
	store := settings.NewStore(serverDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if value, ok := store.Get("AGEZT_EMAIL_SMTP_ADDR#work"); !ok || value != "smtp.owned.test:587" {
		t.Fatal(value, ok)
	}
	if _, ok := store.Get("AGEZT_EMAIL_PASSWORD#work"); ok {
		t.Fatal("secret routed to config")
	}
	vault := creds.NewStore(serverDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if vault.Get("AGEZT_EMAIL_PASSWORD#work") != "owned-sensitive-middle" {
		t.Fatal("selected vault missing trimmed secret")
	}
	if _, err := os.Stat(settings.NewStore(kernelDir).Path); !os.IsNotExist(err) {
		t.Fatal("writer used kernel config", err)
	}
	if os.Getenv("AGEZT_EMAIL_SMTP_ADDR#work") != "" || os.Getenv("AGEZT_EMAIL_PASSWORD#work") != "" {
		t.Fatal("restart writer changed process environment")
	}
	store.Set("AGEZT_EMAIL_PASSWORD#work", "duplicate")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	removed, err := svc.Remove(ctx, appchannels.RemoveAccountInput{Kind: " EMAIL ", Label: " work "})
	if err != nil || !reflect.DeepEqual(removed, appchannels.RemoveAccountOutput{Kind: "email", Label: "work", Removed: 3, Applied: "restart"}) {
		t.Fatal(out, err)
	}
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("AGEZT_EMAIL_SMTP_ADDR#work"); ok || vault.Has("AGEZT_EMAIL_PASSWORD#work") {
		t.Fatal("selected account not removed")
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("direct writer audit/provider effects")
	}
}
