// SPDX-License-Identifier: MIT

package controlplane_test

// Catalog sync must trigger a FULL provider reload (M928), not just a catalog
// snapshot refresh. The regression this guards: a daemon that boots catalog-less
// degrades to the offline mock primary; syncing the catalog from the UI then
// left the mock serving every run (chat answered "[offline-mock]" until the
// scripted responses ran out) even though vault keys made real providers
// eligible — only a daemon restart recovered.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// syncFixtureAPI is a minimal models.dev-shaped payload the sync handler can
// parse and persist.
const syncFixtureAPI = `{
  "testprov": {
    "id": "testprov",
    "name": "Test Provider",
    "env": ["TESTPROV_API_KEY"],
    "npm": "@ai-sdk/openai-compatible",
    "api": "https://api.testprov.invalid/v1",
    "models": {
      "test-model-1": {
        "id": "test-model-1", "name": "Test Model 1", "family": "test",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  }
}`

func TestCatalogSync_TriggersProviderReload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(syncFixtureAPI))
	}))
	defer ts.Close()

	var reloads atomic.Int32
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(),
		Tools:    map[string]agent.Tool{"shell": shell.New()},
		OnReload: func() error { reloads.Add(1); return nil },
	})

	res, err := c.Call(context.Background(), controlplane.CmdCatalogSync, map[string]any{"url": ts.URL})
	if err != nil {
		t.Fatalf("catalog sync: %v", err)
	}
	if got := reloads.Load(); got != 1 {
		t.Errorf("OnReload called %d times after catalog sync, want 1 (providers must re-select against the fresh catalog)", got)
	}
	if reloaded, _ := res["providers_reloaded"].(bool); !reloaded {
		t.Errorf("providers_reloaded = %v, want true: %v", res["providers_reloaded"], res)
	}
	if res["provider_count"] != float64(1) {
		t.Errorf("provider_count = %v, want 1", res["provider_count"])
	}
}

// TestCatalogSync_ProviderReloadFailureIsNonFatal: the catalog snapshot has
// already installed when the provider rebuild fails, so the sync the operator
// asked for must still succeed — with the rebuild error surfaced in the result.
func TestCatalogSync_ProviderReloadFailureIsNonFatal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(syncFixtureAPI))
	}))
	defer ts.Close()

	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(),
		Tools:    map[string]agent.Tool{"shell": shell.New()},
		OnReload: func() error { return context.DeadlineExceeded },
	})

	res, err := c.Call(context.Background(), controlplane.CmdCatalogSync, map[string]any{"url": ts.URL})
	if err != nil {
		t.Fatalf("catalog sync should succeed when only the provider rebuild fails: %v", err)
	}
	if reloaded, _ := res["providers_reloaded"].(bool); reloaded {
		t.Errorf("providers_reloaded = true, want false on rebuild failure")
	}
	if res["provider_reload_error"] == nil {
		t.Errorf("provider_reload_error missing from result: %v", res)
	}
}
