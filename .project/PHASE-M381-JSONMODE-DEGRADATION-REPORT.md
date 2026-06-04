# M381 — Journal JSON-mode capability degradation (SPEC-15 §2.3)

## SPEC audit (read-vs-code)
SPEC-15 §2.3 (and the M376 deferral note) calls for capability **degradation** to
be auditable, not silent.

**Verified gap (grep + read, not assumed):**
- Tool-use degradation IS journaled: the Governor emits `capability.rerouted`
  (M37 down-route) and `capability.rejected` (M25 strict gate) for tools-bearing
  requests to tool-incapable models.
- JSON mode is the hole. `catalog.FamilySupportsNativeJSONMode(family)` exists and
  its own doc says non-native families "ignore a JSON-mode request and rely on a
  prompt-instructed-JSON fallback instead" — but that function was only used by
  `agt check --caps`, never in the live request path. The provider layer confirms
  it: e.g. `plugins/providers/anthropic/anthropic.go` has **no** `JSONMode`
  handling at all, so a `JSONMode` request to an Anthropic-family model is
  silently dropped. Nothing journaled the downgrade — an operator inspecting a
  run could not tell the structured-output request was skipped. Priority-A
  (correctness/observability), offline-verifiable.

## What
- **`kernel/event/kinds.go`** — new `KindCapabilityDegraded = "capability.degraded"`
  (+ knownKinds): a SILENT downgrade that proceeds (vs rejected/rerouted).
- **`kernel/governor/governor.go`** — new injected `Config.ModelJSONNative
  func(model) (native, known bool)`. In `Complete`, after the tool-capability
  gate (so it sees the FINAL resolved model): if `req.JSONMode` and the catalog
  KNOWS the model is non-native, publish `capability.degraded`
  `{model, capability:"json_mode", reason}` carrying the run's `CorrelationID`.
  Non-fatal: the request proceeds on the requested model. An UNKNOWN model is
  never flagged (fail-safe — don't journal what we can't confirm).
- **`cmd/agezt/main.go`** — wires `ModelJSONNative` from the catalog
  (`FindModel` → `Provider.Family()` → `FamilySupportsNativeJSONMode`).

## Verification
- **`kernel/governor/capability_degraded_test.go`** (5 tests, capturing bus +
  real journal): non-native model → exactly one `capability.degraded` AND the
  provider is still called (degradation, not block); payload names
  `json_mode`/model/reason; native model → no event; unknown model → no event;
  JSON mode off → no event; the event carries the request's correlation id.
- **Negative control:** forcing the publish condition to never fire →
  `TestCapabilityDegraded_JSONModeOnNonNativeModel` FAILs (count 0, want 1);
  restored `governor.go` byte-identical.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  green (+5). CHANGELOG (Added, operator-visible journal event).

## Scope notes
- Demo proof is the capturing-fake governor test (real `bus` + `journal`,
  exercises the actual `Complete` path and captures the published event) — the
  goal's allowed "offline / capturing-fake" mode. A live daemon demo would need a
  non-native model present in the catalog (the auto-picked mock provider's "mock"
  model is unknown to the catalog → correctly not flagged), so it is not forced.
- **Follow-up recorded:** the existing `capability.rerouted` / `capability.rejected`
  events do NOT set `CorrelationID` (same orphaning class fixed for warden in
  M379) — a candidate lock-in. And rendering `capability.degraded` in the web UI
  run-detail card (like isolation M379 / policy M380) is a natural next surface.
- Vision capability is gated separately (M91 — a non-vision model never receives
  images), so it is not a silent-degradation gap.
