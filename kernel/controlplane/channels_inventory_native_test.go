// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelInventoryNativeSelectedServerRootAndPresenceOnly(t *testing.T) {
	builtinchannels.RegisterAll()
	t.Setenv(creds.PassphraseEnvVar, "")
	t.Setenv(creds.AutoEncryptEnvVar, "off")
	t.Setenv("AGEZT_EMAIL_SMTP_ADDR", "")
	t.Setenv("AGEZT_EMAIL_PASSWORD", "")
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
	store := settings.NewStore(serverDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.Set("AGEZT_EMAIL_SMTP_ADDR#work", " selected stored ")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	vault := creds.NewStore(serverDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("AGEZT_EMAIL_PASSWORD#work", "owned-sensitive-middle-value"); err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, serverDir)
	s.configEnvPinned = map[string]bool{"AGEZT_EMAIL_SMTP_ADDR": true}
	head, hash := k.Journal().Head()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := s.channelInventory().List(ctx, appchannels.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range out.Channels {
		if row.Kind != "email" {
			continue
		}
		accounts := row.Accounts
		for _, account := range accounts {
			if account.Label != "work" {
				continue
			}
			found = true
			fields := account.Fields
			for _, field := range fields {
				if field.Env == "AGEZT_EMAIL_SMTP_ADDR" && ((field.Value == nil || *field.Value != " selected stored ") || field.EnvPinned != false) {
					t.Fatal(field)
				}
				if field.Env == "AGEZT_EMAIL_PASSWORD" {
					if field.Set != true {
						t.Fatal(field)
					}
					if field.Value != nil {
						t.Fatal("secret value field")
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("selected server account missing")
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "sensitive-middle") {
		t.Fatal("secret content emitted")
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("direct inventory audit/provider effects")
	}
}
