# M370 — Surface anomaly auto-halt in the system changelog (SPEC-08 §4.2)

## SPEC audit (read-vs-code)
SPEC-08 §4.2 (System changelog) defines `agt changelog --system` as a curated,
tamper-evident fold of the journal showing **material changes to this system**,
explicitly including "core update, projection rebuild", "skill
promoted/quarantined/reverted", "trust-ladder change, policy update", and the
halt/resume lifecycle — each entry carrying its event id so `agt why` explains it.

The command exists (M133). `changelogKinds` (the membership set that defines what
counts as "material") covered halt/resume, policy, skill lifecycle, reflection,
catalog sync, and pulse pause/resume.

**Verified gap:** the anomaly auto-halt added this session (M367, SPEC-06 §5)
publishes a `system.anomaly` event and then `HaltWith(reason)`. The `halt` event
was in `changelogKinds` ("system HALTED") but `system.anomaly` was **not** — so
the timeline showed the *symptom* (the system halted) but never the *cause* (the
runaway detection and its reason). For a self-protective auto-halt, the cause is
exactly what an operator needs in the material-change timeline. Genuine SPEC-08
§4.2 gap (priority A — observability of a security action).

## What
- **`kernel/controlplane/changelog.go`** — added `event.KindAnomalyDetected:
  "anomaly auto-halt"` to `changelogKinds`. No other change needed:
  `changelogDetail` already probes the `reason` key, which the anomaly payload
  carries, so the timeline entry renders the human reason automatically.
- Deliberately did NOT add `channel.error` (M358): a recovered per-message
  channel panic is transient operational noise, not a material system-state
  change — it belongs in `agt journal` / `agt doctor`, not the curated timeline.

## Verification
- **`changelog_test.go::TestChangelog_IncludesAnomalyAutoHalt`** (real
  control-plane RPC via startPair): publishes a `system.anomaly` event with a
  reason payload, calls `CmdChangelog`, and asserts it appears with label
  "anomaly auto-halt", the reason as detail, and its event id (for `agt why`).
  The existing `TestChangelog_FiltersToMaterialChanges` still passes (additive).
- **Live daemon demo** (mock, ceiling 1): a 2-tool-call run auto-halted, and
  `agt changelog` showed BOTH lines newest-first — `system HALTED` and
  `anomaly auto-halt`, each with the reason and event id.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2123** passing (was 2122; +1), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-08 audit: §4.1 component-version aggregation is honestly deferred (needs
  the plugin install/update infrastructure that isn't built — `--system` is the
  default and only view today, documented in the command). §4.2 system timeline
  now covers the auto-halt cause. §1 contributions / §2 migrations / §3 update
  mechanism are larger Phase-6-8 features (plugin OCI distribution, migration
  runner) — not gaps to close in one milestone; recorded for honest tracking.
