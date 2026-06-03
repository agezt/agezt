# M271 — `agt skill registry --install <name>` + a skill-import fix

## Why
M268–M270 built the local marketplace loop (export → registry → import), but
installing still meant copy-pasting a bundle path. This milestone adds the
convenience capstone — install by name from a directory registry — and, while
proving it, surfaced and fixed a real latent bug in `agt skill import`.

## What
- **`cmd/agt/skill_registry.go`** — `agt skill registry <dir> --install <name>`:
  - `installFromRegistry(entries, name, …)` resolves a name to exactly one
    **verified** bundle and installs it by delegating to `cmdSkillImport` (which
    re-verifies and dials). It refuses an **ambiguous** name (several verified
    bundles — e.g. different versions — listed so the operator imports the
    intended one by path), reports **no verified bundle** for an absent name, and
    refuses a name whose only candidates are **tampered/malformed**.
- **`cmd/agt/skill_import.go`** — **fix**: the optional `triggers` /
  `tools_required` call args were always sent, so a skill with none sent a JSON
  `null`, which the daemon's strict array decoder rejected ("must be an array").
  They are now included only when non-empty, so a minimal skill imports cleanly.
  (Latent since M269; only hit when a bundle had no triggers/tools.)

## Files
- `cmd/agt/skill_registry.go` — `installFromRegistry` + `--install` flag (edited).
- `cmd/agt/skill_import.go` — omit empty optional list args (edited).
- `cmd/agt/skill.go` — help line for the `--install` form (edited).
- `cmd/agt/skill_registry_test.go` — 1 test (new): name resolution errors
  (ambiguous / only-tampered / absent / parse-error) all exit non-zero with the
  right message, without dialing.
- `kernel/controlplane/skill_test.go` — 1 test (new): import with `triggers` /
  `tools_required` omitted installs a draft at the right content address
  (regression guard for the fix).

## Verification
- `go test ./cmd/agt/ ./kernel/controlplane/ -run 'Skill'` — green; full suite
  **1867 → 1871** (+4), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven** across two homes: exported a skill with **no tools** into a
  registry dir, then in a fresh home `agt skill registry <dir> --install
  diagnose-ci` installed it as a draft (this is the exact path that first
  reproduced the null-args bug, now fixed); an unknown name reported "no verified
  bundle".

## Scope notes
- The marketplace loop is now end-to-end ergonomic: discover *and* install from a
  directory by name, with verification at every step (registry scan + import
  re-verify).
- The fix hardens `agt skill import` for any bundle lacking triggers/tools,
  independent of the registry feature.
