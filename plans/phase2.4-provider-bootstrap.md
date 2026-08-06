# Phase 2.4 — Provider bootstrap (working plan, survey 2026-08-06)

Goal: plugins/providerboot owns Boot+Reload on ONE shared path; retires the boot-vs-reload
drift class (the documented M928/M816 bugs + 4 LIVE drifts found in survey).

## Live drift bugs found (fix as part of the extraction, each with a pinning test)
1. Middleware silently dropped on reload: buildGovernor wraps primary+alternates in
   agent.Wrap(providerMiddleware()); reconcileAlternateProviders + gov.Replace pass raw
   providers → GEN_TEMPERATURE/EXTRACT_REASONING/SIMULATE_STREAMING stop applying after any
   provider reload until restart.
2. Cross-provider down-route eligibility frozen at boot: altFinder closes over the
   `registered` map built in buildGovernor; reload mutates registry but not the map.
3. First-run nudge inverted: main.go:1357 `if model == "mock"` — unconfigured sentinel
   returns "" and mock only comes from DEMO_ECHO=1 → "setup needed" banner fires exactly
   backwards. Fix: check primary name == unconfigured sentinel.
4. Governor env knobs boot-only (RATE_PER_MIN, TASK_ROUTES, MODEL_STRICT, ...): decision =
   do NOT re-read on Reload in this PR (a malformed live edit must not fail reload);
   follow-up ApplyConfig later. Chains have their own live path (chainsSetter) — untouched.

## Key facts
- The block: main.go:4826-5361 (providerMiddleware, buildGovernor, reconcileAlternateProviders,
  catalogModelIDs, selectPrimary, demoEchoProvider, buildFromCatalog) + chatgpt_provider.go
  (registerChatGPTAlternate already has the replace bool) + unconfiguredProvider (5645) +
  awschain.go lookup builders (STAY in cmd/agezt).
- OnReload closure main.go:838-903 duplicates: cred/cat re-Load, lookup rebuild, selectPrimary
  (shared ok), alternates loop (2nd impl), sentinel Remove (reload-only!), SetModel.
  Ordering LOAD-BEARING: registry mutations BEFORE gov.Replace(primary) (Replace rebuilds
  primary/fallback slices from registry).
- Eligibility predicate compat.IsSupportedFamily && HasCredentials copied 4× in main.go
  (buildGovernor, reconcile, cfg.VisionModel :911, cfg.ModelAvailable :930) — fold into
  providerboot.Eligible; rewrite VisionModel/ModelAvailable on it.
- Placeholder-construct (boot-resilient) pattern in buildFromCatalog:5310-5346 must survive
  byte-for-byte (test TestBuildFromCatalog_CrossProviderModelDoesNotFailBoot).
- Hard-fail set (9 error paths in buildGovernor) unchanged for Boot; Reload does NOT
  inherit env-knob failures it doesn't re-read.
- Package: plugins/providerboot (concrete: imports compat/mock/openairesponses/chatgptauth —
  NOT kernel/, would add kernel→plugins edge).

## API
Deps{Catalog, Lookup, BaseDir, Get (nil→os.Getenv), Stderr (nil→io.Discard)}
Result{Governor, Primary, Model, Desc, AuthMode, Eligible}
Boot(d) (*Result, error); Reload(g *governor.Governor, d) (model string, err error)
SelectPrimary/BuildFromCatalog/Eligible/Middleware exported for tests+agt check.
Internal seam: registerAlternates(reg, d, primaryName, mw, replace bool) map[string]bool —
called by Boot(replace=false) and Reload(replace=true → then stale-drop sweep).
Eligible set for altFinder: mutex-guarded map the closure reads, refreshed by Reload.
governorConfigFromEnv(get) split out (2.5 daemonconfig adopts later).

## cmd/agezt after
boot_providers.go: providerDeps(cat, credStore, baseDir, stderr) rebuilding the
catalog-scoped lookup; Boot at :250; OnReload ≈12 lines (credStore.Load, redactor
re-seed, catStore.Load, providerboot.Reload, k.SetModel).

## Tests
- Move: reconcile_providers_test, main_test:326/367/397/427, coverage_chatgpt_test →
  providerboot. awschain_test stays.
- NEW TestBootReloadParity: 3 keyed + 1 unkeyed catalog; Boot snapshot vs
  (unconfigured-boot + Reload) snapshot EQUAL {names, models, authmode, middleware-wrapped}.
  Add Snapshot helper. This is the test M928+M816 never had.
- NEW: middleware survives Reload; Eligible updates across Reload; sentinel promoted to
  primary[0]; alternates-before-Replace order.
- Fix + assert the first-run nudge polarity in cmd/agezt.
- REPOINT kernel/controlplane/config_inventory_test scan roots to
  [cmd/agezt, plugins/providerboot, plugins/builtinchannels, plugins/builtintools] —
  2.1/2.2 already opened this blind spot; close it here.
- Run: cmd/agezt, kernel/controlplane, kernel/runtime, kernel/governor, providerboot.
