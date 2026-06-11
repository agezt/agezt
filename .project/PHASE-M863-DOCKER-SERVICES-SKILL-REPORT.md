# PHASE M863 — Built-in docker-services skill bundle

**Status:** shipped
**Milestone:** M863 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Backlog #51 — *agents can run self-hosted services in the background
via Docker* — delivered conflict-free as a fourth out-of-the-box skill bundle.

## Approach

Agents already have full machine capability (the `shell` and `code_exec` tools,
default-allow posture) — they *can* already run `docker`. What was missing was
the **lifecycle discipline**: a consistent way to start services so they survive
reboots, stay discoverable, and don't become orphaned junk. That is exactly what
a skill bundle provides — knowledge + a helper script — with no kernel plumbing.

## What shipped

A built-in `docker-services` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only):

- `SKILL.md` — when to stand up a real background service vs a throwaway script,
  the **one rule** (label every service `agezt.service=1` so agezt's containers
  are always findable/reapable without touching the user's own), the docker
  availability check, the lifecycle, and how to connect/persist/clean up.
- `scripts/svc.sh` — a POSIX lifecycle helper enforcing the label + `agezt-<name>`
  naming: `up` (idempotent — reuses a running container, starts a stopped one,
  else creates with `--restart unless-stopped`), `down`, `nuke` (down + remove
  named volumes), `ls` (only agezt services), `logs`, `ip`.
- `reference/services.md` — ready recipes (Postgres, Redis, MinIO, Ollama,
  Meilisearch, n8n) with ports, named volumes, and connection strings, plus
  health-poll and compose guidance.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor` (the files a concurrent session is actively
editing for M858/M859). The seeder auto-loads it. It tests in isolation:
`go test ./plugins/builtinskills/` compiles just this package + `kernel/skill`, so
verification never compiles the in-flux Go kernel.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `sh -n svc.sh` syntax-clean (shellcheck not installed in
  this env). Package test suite green — `TestSeedAll_InstallsDockerServices` asserts
  the bundle seeds **active** and materializes `scripts/svc.sh` + `reference/services.md`;
  the existing `len(seeded) == len(builtinBundles)` assertions now cover four bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` / daemon smoke deliberately
  skipped — they'd compile the concurrent in-progress Go edits.

## Notes
- Four seeded bundles now ship out of the box (browser-use, computer-use,
  data-analysis, docker-services). The `agezt.service=1` label is the contract a
  future automated reaper (#53) can use to collect dead services.
