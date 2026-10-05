// SPDX-License-Identifier: MIT

package providers_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestProviderConnectPreservesCatalogAndUnknownModels(t *testing.T) {
	var reloads atomic.Int32
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if err := k.CatalogStore().SaveAPI([]byte(`{"known":{"id":"known","models":{"a":{"id":"a"},"b":{"id":"b"}}}}`), "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.ReloadCatalog(); err != nil {
		t.Fatal(err)
	}
	svc := providers.New(k, dir)
	result, err := svc.Connect(context.Background(), providers.ConnectInput{ID: "known", API: "https://fixture.invalid", Model: "invented"})
	if err != nil {
		t.Fatal(err)
	}
	if result["exists"] != true || result["added"] != false || len(k.Catalog().Providers["known"].Models) != 2 {
		t.Fatalf("existing provider clobbered: %v", result)
	}
	if _, err := os.Stat(filepath.Join(k.CatalogStore().Dir, catalog.FileCustom)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("existing id wrote custom catalog: %v", err)
	}
	result, err = svc.Connect(context.Background(), providers.ConnectInput{ID: " new ", API: " https://new-fixture.invalid ", Model: "hint-only"})
	if err != nil {
		t.Fatal(err)
	}
	created := k.Catalog().Providers["new"]
	if result["exists"] != false || result["added"] != true || created == nil || len(created.Models) != 0 || created.Name != "new" || created.NPM != "@ai-sdk/openai-compatible" {
		t.Fatalf("new provider/model/defaults=%v %+v", result, created)
	}
	reloaded, err := svc.Reload(context.Background(), providers.ReloadInput{})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded["providers_reloaded"] != true || reloaded["provider_count"] != 2 || reloads.Load() != 3 {
		t.Fatalf("reload result=%v count=%d", reloaded, reloads.Load())
	}
	if _, err := svc.Connect(context.Background(), providers.ConnectInput{ID: "invalid", API: "https://fixture.invalid", Env: brand.EnvPrefix + "CONFIG"}); err == nil {
		t.Fatal("config namespace accepted as provider credential")
	}
}

func TestProviderKeyringLifecyclePrivacyAndScope(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-provider-fixture")
	var reloads atomic.Int32
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	svc := providers.New(k, dir)
	ctx := context.Background()
	secret := strings.Join([]string{"fixture", "private", "value", "1234"}, "-")
	input := providers.KeyInput{Provider: "alpha", Env: "PROVIDER_FIXTURE_KEY", Label: "first", Value: secret}
	added, err := svc.KeyAdd(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if added["active_changed"] != true || reloads.Load() != 1 {
		t.Fatalf("first key activation=%v %d", added, reloads.Load())
	}
	input.Label = "second"
	input.Value = secret + "-5678"
	added, err = svc.KeyAdd(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if added["active_changed"] != false || reloads.Load() != 1 {
		t.Fatalf("inactive add reloaded provider: %v %d", added, reloads.Load())
	}
	listed, err := svc.KeyList(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("key value left provider service")
	}
	if len(listed["keys"].([]creds.KeyInfo)) != 2 {
		t.Fatalf("key list=%v", listed)
	}
	other, err := svc.KeyList(ctx, providers.KeyInput{Provider: "beta", Env: input.Env})
	if err != nil {
		t.Fatal(err)
	}
	if len(other["keys"].([]creds.KeyInfo)) != 0 {
		t.Fatal("scoped keyring leaked to another provider")
	}
	if _, err := svc.KeyActivate(ctx, input); err != nil {
		t.Fatal(err)
	}
	if reloads.Load() != 2 {
		t.Fatal("activate did not reload")
	}
	input.Label = "first"
	removed, err := svc.KeyRemove(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if removed["was_active"] != false || reloads.Load() != 2 {
		t.Fatalf("inactive removal reloaded=%v %d", removed, reloads.Load())
	}
	input.Label = "second"
	removed, err = svc.KeyRemove(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if removed["was_active"] != true || reloads.Load() != 3 {
		t.Fatalf("active removal=%v %d", removed, reloads.Load())
	}
	vault := creds.NewStore(dir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if vault.Get(catalog.ProviderCredentialName("alpha", input.Env)) != "" {
		t.Fatal("active mirror persisted after removal")
	}
	if _, err := svc.KeyList(ctx, providers.KeyInput{Provider: "../invalid", Env: input.Env}); err == nil {
		t.Fatal("invalid provider id accepted")
	}
}

func TestProviderReloadFailureKeepsSuccessfulChanges(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-provider-fixture")
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { return context.DeadlineExceeded }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	svc := providers.New(k, dir)
	connected, err := svc.Connect(context.Background(), providers.ConnectInput{ID: "new", API: "https://fixture.invalid"})
	if err != nil || connected["reload_error"] == nil {
		t.Fatalf("connect rebuild failure=%v %v", connected, err)
	}
	connected, err = svc.Connect(context.Background(), providers.ConnectInput{ID: "new", API: "https://changed-fixture.invalid"})
	if err != nil || connected["exists"] != true || connected["reload_error"] == nil {
		t.Fatalf("existing provider rebuild failure=%v %v", connected, err)
	}
	added, err := svc.KeyAdd(context.Background(), providers.KeyInput{Env: "PROVIDER_FIXTURE_KEY", Label: "fixture", Value: "synthetic-fixture-value"})
	if err != nil || added["reload_error"] == nil {
		t.Fatalf("key rebuild failure=%v %v", added, err)
	}
	if _, err := svc.Reload(context.Background(), providers.ReloadInput{}); err == nil {
		t.Fatal("explicit reload hid provider rebuild failure")
	}
}

func TestProviderReloadWithoutRebuildKeepsOperatorNote(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	result, err := providers.New(k, dir).Reload(context.Background(), providers.ReloadInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result["providers_reloaded"] != false || !strings.Contains(result["note"].(string), "OnReload not configured") {
		t.Fatalf("catalog-only reload lost note: %v", result)
	}
}
