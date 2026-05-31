# Phase Report — Milestone M23 (Model capability inspection)

> Status: **shipped** · Date: 2026-05-31
> SPEC-15 §1 (catalog). The provider catalog already carried rich per-model
> capability flags; nothing read them. M23 surfaces them and flags the one that
> matters most for an autonomous agent — tool-use — before a run, with no
> network call.

## Why

After nine milestones on the policy/security axis (M14–M22), this turn moved to
the mandate's other explicit promise: "works with every kind of provider/model."
A read-only survey of the provider layer found it genuinely mature — 9 providers,
10 wire families, streaming everywhere, and a catalog (`kernel/catalog`) that
already tracks `tool_call`, `reasoning`, `attachment`, `modalities`, and token
`limit` per model. But that capability data was **inert**: no command reported
it, and nothing checked it. The concrete failure mode: this system's agent loop
is fundamentally tool-driven, so pointing it at a model whose `tool_call=false`
(a small local model, a text-only endpoint) fails deep inside a run with a
cryptic upstream 400 instead of a clear "this model can't call tools" up front.

`agt provider check` already existed but only does a **live probe** (latency,
cost, streaming, bench) — it needs credentials and a network round-trip, and it
never reported what a model can *do*. The gap was a static, pre-flight capability
view.

## What shipped

- **Pure catalog helpers (`kernel/catalog/types.go`)**:
  - `Model.SupportsModality(io, name)` — case-insensitive input/output modality
    check.
  - `Model.SupportsVision()` — convenience for image input (accepts both the
    `image` and `vision` spellings catalogs use).
  - `Model.AgentWarnings()` — operator-facing advisories about a model's fitness
    for the tool-driven loop. Headline: no tool-use (`tool_call=false`). Also
    flags a small context window (< 8192). The wording says a model "does not
    *advertise*" tool-use, because the signal is catalog metadata — a local model
    may support tools without the catalog knowing, so this informs rather than
    blocks.
- **`agt provider check --caps [<id>]` (`cmd/agt/check.go`)** — a new mode that
  reports a model's capabilities **with no network call and no credentials**
  (capabilities are static catalog facts). Resolves the provider/model the same
  way as a probe (explicit id, `$AGEZT_PROVIDER`, or first supported family; no
  credential filter) and prints tool-use / reasoning / vision / attachments /
  input+output modalities / context+output limits / knowledge cutoff, then any
  `AgentWarnings` under a ⚠ marker (or a ✓ agent-ready line). `--caps --json`
  emits a stable `jsonCaps` record. **Exit 3** when warnings exist, so CI can
  gate "is the model I'm about to deploy agent-ready?" without parsing text;
  rejects combination with `--all`/`--bench`/`--stream`.

No `go.mod` change. No daemon changes (a static catalog inspection on the CLI
side). No new control-plane command.

## Proven

- **Unit (catalog):** `SupportsModality` (case-insensitivity, input vs output,
  unknown io), `SupportsVision` (image/vision spellings, text-only negative),
  `AgentWarnings` (tool-capable → none; no-tools → tool warning; no-tools +
  tiny-context → two; tool-capable + tiny-context → context warning only).
- **Unit (CLI):** `--caps`/`--capabilities` flag parsing; `runCheckCaps` against
  a synthetic catalog — a tool-less model prints `tool-use: no`, `vision: yes`,
  the ⚠ tool-use warning, and exits 3; the `--json` form unmarshals to the
  expected `jsonCaps`; an unknown provider exits 1.
- **Live (network-free, no creds):** with a hand-written `custom.json` catalog,
  `agt provider check acme --caps` on a `tool_call=false` model prints the full
  capability block + the ⚠ tool-use advisory and **exits 3**; the same on a
  tool-capable model prints `✓ agent-ready` and **exits 0**; `--caps --json`
  emits the structured record (`tool_call:false`, `vision:true`, `warnings:[…]`);
  `--caps --all` is rejected (exit 2).

6 new tests; suite **1190** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — named

- **Boot-time advisory** — the daemon could emit the same tool-use warning in its
  startup banner when the auto-selected primary model lacks tool-use (uses the
  same `AgentWarnings`). Held back from M23 to avoid false alarms on
  under-flagged local models; a conservative "advertise" banner line is the
  natural M24 follow-up.
- **Request-time capability validation** in the Governor (reject a tools request
  to a non-tool model with a clear pre-flight error) — higher-value but riskier
  given catalog-data-quality variance; wants the advisory path proven first.
- **`--caps --all`** — a capability matrix across every cataloged model, for
  picking a model by capability rather than probing one at a time.
- **Vision/attachment validation** once the agent message type carries image
  content end-to-end (today the catalog tracks the modality; the loop is text).

## Arc

M23 turns to the "every provider/model" axis after the M14–M22 policy arc. The
catalog's capability data is now observable and the agent-readiness of a chosen
model is a one-command, network-free, CI-gatable check — closing the gap between
"the catalog knows" and "the operator is told."
