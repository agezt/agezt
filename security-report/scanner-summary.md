# Scanner and Verification Summary

Date: 2026-06-24

## Commands run

| Command | Result |
|---|---|
| `gitleaks detect --no-git --redact --config .gitleaks.toml --report-format json --report-path security-report/gitleaks.json --exit-code 0` | Pass - no leaks found after adding `.env.example` |
| `go mod verify` | Pass - all modules verified |
| `govulncheck ./...` | Pass - no vulnerabilities found |
| `go vet ./...` | Pass |
| `staticcheck ./...` | Non-security lint issues only |
| `gosec -fmt=json ./...` | No High/Critical confirmed after manual triage; noisy findings reviewed |
| `npm audit --json` in `frontend` | Pass - 0 vulnerabilities |
| `npm run typecheck` in `frontend` | Pass |
| `npm audit --json` in `sdk/typescript` | Pass - 0 vulnerabilities |
| `npm test -- --runInBand` in `sdk/typescript` | Pass - 14 tests |
| `cargo metadata --manifest-path sdk/rust/Cargo.toml --no-deps` | Pass - Rust SDK has no runtime dependencies |
| `go test ./kernel/controlplane ./kernel/webui ./kernel/restapi ./kernel/openaiapi ./kernel/agentgw ./kernel/envscrub ./plugins/tools/coding ./plugins/tools/acpagent ./plugins/tools/overseertool ./plugins/providers/embed ./plugins/providers/voice` | Pass - remediation smoke test |
| `go test ./internal/ciguard ./kernel/streamlimit ./kernel/webui ./kernel/restapi ./plugins/channels/discord` | Pass - CI guard, SSE cap, REST/Web UI, Discord attachment regression tests |
| `go test ./internal/ciguard ./kernel/streamlimit ./kernel/webui ./kernel/restapi ./plugins/channels/discord ./kernel/controlplane ./kernel/openaiapi ./kernel/agentgw ./kernel/envscrub ./plugins/tools/coding ./plugins/tools/acpagent ./plugins/tools/overseertool ./plugins/providers/embed ./plugins/providers/voice` | Pass - combined remediation smoke test |
| `go test ./internal/ciguard` | Pass - self-hosted fork guard, checkout credential persistence, setup-go-safe fallback, Dependabot coverage, and sanitized `.env.example` invariants |
| `.env` secret-shaped value check + ACL inspection | Pass - `.env` values blanked; ACL narrowed to current user, SYSTEM, and Administrators |
| `git check-ignore -v .env.example` | Pass - `.env.example` is explicitly unignored by `.gitignore` |
| `rg --files \| rg "(fix_scout|scout_|update_scout|find_model|find_302ai|readlines|start-daemon)"` | Pass - no reported local root debug/launcher scripts present |
| `go list -m -versions github.com/emersion/go-imap/v2` | Latest listed version is still `v2.0.0-beta.8`; no stable v2 target available |
| `.github/dependabot.yml` added | Go modules, frontend npm, TypeScript SDK npm, and GitHub Actions are now tracked weekly |

## Tooling limitations

- `cargo-audit` was not installed, so Rust advisory scanning was limited to
  dependency inventory. Current Rust SDK metadata shows zero runtime
  dependencies.
- `gosec ./...` scanned generated/temp worktree copies as well as primary source
  paths. Findings were manually triaged against the primary source tree.
- This was a static and local-command audit. It did not include live fuzzing,
  authenticated browser abuse testing, or third-party infrastructure testing.

## Dependency posture

- Go module graph is small: 4 direct modules plus a small indirect set.
- `govulncheck` found no reachable Go vulnerabilities.
- npm audit found no frontend or TypeScript SDK vulnerabilities.
- Python SDK and Rust SDK have no runtime dependency surface in the inspected
  metadata.
- Remaining dependency risk is freshness/maintenance, mainly beta/pseudo/stale
  transitive pins documented in `dependency-audit.md`.

## Static-analysis notes

`staticcheck` reported non-security cleanup items:

- `cmd/agezt/auto_repair_test.go:965` - SA4006 unused assignment to `tail`
- `cmd/agezt/main.go:291` - SA4006 unused assignment to `cat`
- `kernel/memory/epistemic_test.go:65` - SA4000 identical expressions
- `kernel/runtime/runtime.go:2287` - U1000 unused `agentLifecycleFromCtx`

These were not remediated because the requested work was a security scan.

## Report artifacts

- `architecture.md` - attack-surface and trust-boundary map
- `api-client-results.md` - API/client-side auth, CSRF, CORS, clickjacking, SSE review
- `dependency-audit.md` - supply-chain review
- `code-exec-results.md` - code execution and deserialization review
- `injection-results.md` - injection/XSS/header/XXE/SSTI review
- `verified-findings.md` - manually verified findings and rejected false positives
- `gitleaks.json` - redacted machine-readable secret scan output
