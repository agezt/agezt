// SPDX-License-Identifier: MIT

package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

const fixture = `{"fixture":{"id":"fixture","name":"Fixture","env":["CATALOG_FIXTURE_KEY"],"npm":"@ai-sdk/openai-compatible","models":{"z":{"id":"z","name":"Z","tool_call":true},"a":{"id":"a","name":"A","tool_call":true,"limit":{"context":4096,"output":128},"cost":{"input":1,"output":2}}}},"other":{"id":"other","name":"Other","env":["CATALOG_FIXTURE_KEY"],"models":{}}}`

func TestCatalogHandlersWorkWithoutSocketAndRetainWire(t *testing.T) {
	for _, reloadFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "reload", true: "reload-error"}[reloadFails], func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					_, _ = w.Write([]byte(`{"models":[{"name":"local-fixture","model":"local-fixture","details":{"family":"fixture"}}]}`))
					return
				}
				_, _ = w.Write([]byte(fixture))
			}))
			defer ts.Close()
			dir := t.TempDir()
			var reloads atomic.Int32
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error {
				reloads.Add(1)
				if reloadFails {
					return context.DeadlineExceeded
				}
				return nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			svc := appcatalog.New(k, dir)
			t.Setenv(brand.EnvPrefix+"CATALOG_URL", ts.URL)
			t.Setenv(brand.EnvPrefix+"OLLAMA_ENDPOINT", ts.URL)
			t.Setenv("CATALOG_FIXTURE_KEY", "")
			t.Setenv(creds.PassphraseEnvVar, "isolated-catalog-fixture")
			synced, err := svc.Sync(context.Background(), appcatalog.SyncInput{TimeoutSeconds: 1})
			if err != nil {
				t.Fatal(err)
			}
			if synced["url"] != ts.URL || synced["provider_count"] != 2 || synced["model_count"] != 2 || synced["providers_reloaded"] != !reloadFails {
				t.Fatalf("sync result=%v", synced)
			}
			if _, exists := synced["provider_reload_error"]; exists != reloadFails {
				t.Fatalf("reload error presence=%v", synced)
			}
			// Bare vault entries do not credential every provider sharing an env;
			// the fixture's provider-scoped entry credentials exactly that provider.
			vault := creds.NewStore(dir)
			vault.SetPassphraseFn(func() string { return "" })
			if err := vault.Set("CATALOG_FIXTURE_KEY", "fixture-value"); err != nil {
				t.Fatal(err)
			}
			if err := vault.Set(catalog.ProviderCredentialName("fixture", "CATALOG_FIXTURE_KEY"), "scoped-fixture-value"); err != nil {
				t.Fatal(err)
			}
			if err := vault.Save(); err != nil {
				t.Fatal(err)
			}
			before, _ := k.Journal().Head()
			listed, err := svc.List(context.Background(), appcatalog.ListInput{})
			if err != nil {
				t.Fatal(err)
			}
			after, _ := k.Journal().Head()
			if before != after {
				t.Fatal("list changed journal")
			}
			providers := listed["providers"].([]map[string]any)
			if len(providers) != 2 || providers[0]["id"] != "fixture" || providers[0]["credentialed"] != true || providers[1]["credentialed"] != false {
				t.Fatalf("provider/credential projection=%v", providers)
			}
			models := providers[0]["models"].([]map[string]any)
			if len(models) != 2 || models[0]["id"] != "a" || models[1]["id"] != "z" || models[0]["context"] != 4096 || models[0]["cost_input_usd_per_mtok"] != float64(1) {
				t.Fatalf("model projection=%v", models)
			}
			if _, exists := models[1]["cost_input_usd_per_mtok"]; exists {
				t.Fatal("free model acquired a price")
			}
			encoded, err := json.Marshal(listed)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "fixture-value") {
				t.Fatal("credential value reached catalog result")
			}
			discovered, err := svc.Discover(context.Background(), appcatalog.DiscoverInput{})
			if err != nil {
				t.Fatal(err)
			}
			if discovered["endpoint"] != ts.URL || discovered["model_count"] != 1 || discovered["providers_reloaded"] != !reloadFails || reloads.Load() != 2 {
				t.Fatalf("discovery/reloads=%v %d", discovered, reloads.Load())
			}
			if message, exists := discovered["provider_reload_error"]; exists != reloadFails || (exists && !strings.Contains(message.(string), context.DeadlineExceeded.Error())) {
				t.Fatalf("discovery reload error=%v", discovered)
			}
			var kinds []event.Kind
			if err := k.Journal().Range(func(e *event.Event) error {
				if strings.HasPrefix(e.Subject, "catalog.") {
					kinds = append(kinds, e.Kind)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(kinds) != 2 || kinds[0] != event.KindCatalogSynced || kinds[1] != event.KindCatalogDiscoveryCompleted {
				t.Fatalf("domain events=%v", kinds)
			}
		})
	}
}

func TestCatalogFailedFetchRetainsEventsAndSkipsPersistenceReload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	var reloads atomic.Int32
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	svc := appcatalog.New(k, dir)
	if output, err := svc.Sync(context.Background(), appcatalog.SyncInput{URL: ts.URL}); err == nil || output != nil {
		t.Fatalf("failed sync=%v %v", output, err)
	}
	if output, err := svc.Discover(context.Background(), appcatalog.DiscoverInput{Endpoint: ts.URL}); err == nil || output != nil {
		t.Fatalf("failed discovery=%v %v", output, err)
	}
	meta, err := k.CatalogStore().LoadMeta()
	if err != nil {
		t.Fatal(err)
	}
	if reloads.Load() != 0 || !meta.APISyncedAt.IsZero() || !meta.LocalSyncedAt.IsZero() {
		t.Fatalf("failed fetch changed metadata/reloads: %v %d", meta, reloads.Load())
	}
	var kinds []event.Kind
	if err := k.Journal().Range(func(e *event.Event) error {
		if strings.HasPrefix(e.Subject, "catalog.") {
			kinds = append(kinds, e.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 || kinds[0] != event.KindCatalogSyncFailed || kinds[1] != event.KindCatalogDiscoveryFailed {
		t.Fatalf("failed domain events=%v", kinds)
	}
}
