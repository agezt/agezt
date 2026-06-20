// SPDX-License-Identifier: MIT

package main

// reconcileAlternateProviders (M928) keeps the hot-reload registry in parity
// with the boot path: every credentialed+supported catalog provider is
// registered as a model-routable alternate (with its catalog Models, so
// per-task model chains route each model to its serving provider), and
// alternates that lost eligibility are dropped. Regression guarded: after the
// first-run flow (boot catalog-less → mock primary, then catalog sync + keys +
// reload) only the primary was swapped in, so chain models from OTHER keyed
// providers kept failing until a daemon restart.

import (
	"slices"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/mock"
)

const reconcileFixtureAPI = `{
  "alpha": {
    "id": "alpha",
    "name": "Alpha",
    "env": ["ALPHA_API_KEY"],
    "npm": "@ai-sdk/openai-compatible",
    "api": "https://api.alpha.invalid/v1",
    "models": {
      "alpha-large": {
        "id": "alpha-large", "name": "alpha-large", "family": "alpha",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  },
  "beta": {
    "id": "beta",
    "name": "Beta",
    "env": ["BETA_API_KEY"],
    "npm": "@ai-sdk/openai-compatible",
    "api": "https://api.beta.invalid/v1",
    "models": {
      "beta-mini": {
        "id": "beta-mini", "name": "beta-mini", "family": "beta",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  },
  "unkeyed": {
    "id": "unkeyed",
    "name": "Unkeyed",
    "env": ["UNKEYED_API_KEY"],
    "npm": "@ai-sdk/openai-compatible",
    "api": "https://api.unkeyed.invalid/v1",
    "models": {
      "unkeyed-1": {
        "id": "unkeyed-1", "name": "unkeyed-1", "family": "unkeyed",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  }
}`

func reconcileLookup(name string) string {
	switch name {
	case "ALPHA_API_KEY", "BETA_API_KEY":
		return "test-key"
	}
	return ""
}

func TestReconcileAlternateProviders_RegistersKeyedAlternates(t *testing.T) {
	cat, err := catalog.ParseAPIFile([]byte(reconcileFixtureAPI))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	// Boot-with-empty-catalog state: the mock is the (non-fallback) primary
	// and nothing else is registered.
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "mock", Provider: mock.New(), AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatal(err)
	}

	// Reload selected "alpha" as the new primary (the caller installs it via
	// gov.Replace afterwards); reconcile must register "beta" as an alternate
	// with its catalog model list and leave "unkeyed" out.
	reconcileAlternateProviders(reg, cat, reconcileLookup, "alpha", t.TempDir())

	beta, ok := reg.Get("beta")
	if !ok {
		t.Fatal("beta (keyed+supported) not registered as an alternate")
	}
	if !slices.Contains(beta.Models, "beta-mini") {
		t.Errorf("beta.Models = %v, want to contain beta-mini (per-request model routing needs it)", beta.Models)
	}
	if _, ok := reg.Get("unkeyed"); ok {
		t.Error("unkeyed provider registered despite having no credentials")
	}
}

func TestReconcileAlternateProviders_DropsStaleAlternates(t *testing.T) {
	cat, err := catalog.ParseAPIFile([]byte(reconcileFixtureAPI))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	reg := governor.NewRegistry()
	// A previously registered alternate whose key has since been revoked
	// (not credentialed under the current lookup) must be dropped...
	if err := reg.Register(&governor.ProviderInfo{
		Name: "stale-prov", Provider: &alwaysFailProvider{name: "stale-prov"}, AuthMode: governor.AuthAPIKey,
	}); err != nil {
		t.Fatal(err)
	}
	// ...while a FALLBACK entry is never touched (the demoted offline mock).
	if err := reg.Register(&governor.ProviderInfo{
		Name: "mock", Provider: mock.New(), AuthMode: governor.AuthLocal, IsFallback: true,
	}); err != nil {
		t.Fatal(err)
	}

	reconcileAlternateProviders(reg, cat, reconcileLookup, "alpha", t.TempDir())

	if _, ok := reg.Get("stale-prov"); ok {
		t.Error("stale alternate (no longer credentialed/in catalog) was not removed")
	}
	if info, ok := reg.Get("mock"); !ok || !info.IsFallback {
		t.Error("fallback mock must survive reconcile untouched")
	}
	if _, ok := reg.Get("beta"); !ok {
		t.Error("beta (keyed+supported) not registered as an alternate")
	}
}
