// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func oauthFixtureFile(t *testing.T) string {
	t.Helper()
	claims, _ := json.Marshal(map[string]any{"email": "fixture@example.invalid"})
	id := "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"
	raw, _ := json.Marshal(map[string]any{"tokens": map[string]any{"access_token": "fixture-access-private", "refresh_token": "fixture-refresh-private", "id_token": id, "account_id": "fixture-account"}})
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProviderOAuthCommandMetadataComesFromAppSpecs(t *testing.T) {
	if len(oauthOperations) != 4 {
		t.Fatal("OAuth registry incomplete")
	}
	for _, operation := range oauthOperations {
		spec := operation.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || wire.ReadOnly != (spec.Name == CmdProviderOAuthStatus) || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("OAuth command metadata=%+v", wire)
		}
		if spec.Name == CmdProviderOAuthImport && spec.Output != reflect.TypeFor[providers.OAuthImportOutput]() {
			t.Fatal("OAuth output is opaque")
		}
	}
}

func TestProviderOAuthAppSocketAuditAndAuthoritativeEmptyModels(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	var reloads, syncs atomic.Int32
	var empty atomic.Bool
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetChatGPTSync(func() ([]string, string) {
		syncs.Add(1)
		if empty.Load() {
			return []string{}, ""
		}
		return []string{"fixture-live-model"}, "fixture-live-model"
	})
	call := func(command string, args map[string]any) Response {
		t.Helper()
		result := callAppHost(t, s, Request{ID: command, Cmd: command, Token: "primary", Args: args})[0]
		if result.Type != RespResult {
			t.Fatalf("%s failed: %+v", command, result)
		}
		return result
	}
	imported := call(CmdProviderOAuthImport, map[string]any{"path": oauthFixtureFile(t), "ignored": true})
	if imported.Result["connected"] != true || imported.Result["default_model"] != "fixture-live-model" || reloads.Load() != 1 {
		t.Fatalf("OAuth import=%v reload=%d", imported, reloads.Load())
	}
	before, _ := k.Journal().Head()
	empty.Store(true)
	status := call(CmdProviderOAuthStatus, map[string]any{"state": "unknown", "ignored": true})
	after, _ := k.Journal().Head()
	if before != after || status.Result["connected"] != true || status.Result["default_model"] != "" || len(status.Result["models"].([]any)) != 0 {
		t.Fatalf("authoritative empty status=%v journal=%d->%d", status, before, after)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-access-private", "fixture-refresh-private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("OAuth response exposed token")
		}
	}
	call(CmdProviderOAuthLogout, nil)
	status = call(CmdProviderOAuthStatus, nil)
	if status.Result["connected"] != false || reloads.Load() != 2 || syncs.Load() != 2 {
		t.Fatalf("OAuth logout/status=%v reload/sync=%d/%d", status, reloads.Load(), syncs.Load())
	}
	var events []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		if strings.HasPrefix(e.Subject, "op.provider_oauth_") {
			events = append(events, e)
			if strings.Contains(string(e.Payload), "fixture-access-private") || strings.Contains(string(e.Payload), "fixture-refresh-private") {
				t.Error("OAuth token entered journal")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("OAuth mutation/read-only audit=%v", events)
	}
	for i := 0; i < len(events); i += 2 {
		if events[i].Kind != event.KindOpInvoked || events[i+1].Kind != event.KindOpCompleted || events[i].CorrelationID == "" || events[i].CorrelationID != events[i+1].CorrelationID {
			t.Fatal("OAuth audit identity/order changed")
		}
	}
}

func TestProviderOAuthAppAdmissionPreventsStateAndVaultEffects(t *testing.T) {
	for _, command := range []string{CmdProviderOAuthStart, CmdProviderOAuthImport, CmdProviderOAuthLogout} {
		t.Run(command, func(t *testing.T) {
			t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
			dir := t.TempDir()
			var reloads atomic.Int32
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			s := NewServer(k, dir)
			s.token = "primary"
			args := map[string]any{"provider": "chatgpt", "path": oauthFixtureFile(t)}
			_ = k.Journal().Close()
			response := callAppHost(t, s, Request{ID: "denied", Cmd: command, Token: "primary", Args: args})[0]
			if response.Type != RespError || s.providerOAuthState != nil || reloads.Load() != 0 {
				t.Fatalf("rejected OAuth reached state/reload: %v state=%v reload=%d", response, s.providerOAuthState, reloads.Load())
			}
			if _, err := os.Stat(creds.NewStore(dir).Path); !os.IsNotExist(err) {
				t.Fatalf("rejected OAuth created vault: %v", err)
			}
		})
	}
}
