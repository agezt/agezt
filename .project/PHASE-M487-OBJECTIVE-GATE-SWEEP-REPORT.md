# M487 — Objective static-analysis & security gate sweep (audit trail)

## Purpose
The DEFECT-HUNT arc (M456–M484) found defects by manual review + pattern scans.
"100% hardening" needs an *objective* terminal condition, not "I stopped finding
bugs." This milestone runs the full offline analysis toolchain installed on the
machine, drives what can be driven to zero, and triages the rest finding-by-finding
so every result is either fixed, enforceable, or classified with reasoning. This
report is the audit trail.

## Toolchain present (verified on PATH / GOPATH/bin)
`go vet`, `staticcheck`, `govulncheck`, `errcheck`, `gosec`, `golangci-lint`,
`shadow`, `deadcode`, `gocyclo`, `gitleaks`, `gofumpt`, `go-mutesting`.

## Results

| Gate | Raw | Disposition |
|------|-----|-------------|
| `go vet ./...` | 0 | **CLEAN** (enforceable) |
| `staticcheck ./...` | 17 → **0** | **FIXED → enforceable** (M485) |
| `gitleaks detect` | 16 → **0** | **BASELINED → enforceable** (M486) |
| `govulncheck ./...` | 2 (stdlib) | Documented; remediation = toolchain bump (below) |
| `errcheck -ignoretests` | 116 | Triaged — all idiomatic intentional-ignores; no bug |
| `deadcode ./...` | 35 | Triaged — all false positives (library API / plugins / tests) |
| `shadow` | ~40 | Triaged — all idiomatic `if err :=` blocks; no error-masking |
| `gosec ./...` | ~60 | Triaged — all false-positive or by-design in threat model |

`go vet`, `staticcheck`, and `gitleaks` are now zero and suitable as CI gates.

## govulncheck — 2 standard-library advisories (real, remediation documented)
Current toolchain: **go1.26.3**. Both are fixed in **go1.26.4**:

- **GO-2026-5039** — `net/textproto`: arbitrary input included in errors without
  escaping. Reached via `smtp.SendMail` (email channel), `bufio.Scanner` MIME header
  read (journal Range), `mcpbridge`.
- **GO-2026-5037** — `crypto/x509`: inefficient candidate hostname parsing. Reached
  via `http.Server.ListenAndServe` (slack) and x509 hostname error formatting.

These live in the compiler-shipped standard library, not in a module dependency, so
there is **nothing in Agezt's code to change** and `go.mod`/`go.sum` are irrelevant
to the fix (and remain unchanged per project policy). **Remediation: build/release
and CI with go ≥ 1.26.4.** go1.26.4 is not fetchable in this offline environment, so
this is recorded as a known advisory with a one-line fix at the next toolchain bump.
After that bump `govulncheck ./...` is expected to report 0.

## errcheck (116) — all idiomatic intentional-ignores
Categories, all standard Go practice:
- `fmt.Fprintf`/`Fprintln` to stdout/stderr (the dominant share) — return ignored by
  universal convention.
- `hash.Hash.Write` (artifact.go, memory.go) — documented to **never** return an error.
- best-effort `Close()` / `os.Remove()` on cleanup & error paths (journal, memory,
  catalog, creds, backup, agt). The durability-critical *success* write paths use
  explicit `Sync()` (hardened M421/M462/M471/M478) before close+rename.
- `go chan.Start(ctx)` — a `go` statement's return cannot be checked; channels log
  their own errors.
- fire-and-forget `Bus().Publish(...)` / pulse `publish(...)` — best-effort by design.
No genuine unchecked-error bug. (Forcing to zero would mean `_ =` on 116 provably-safe
sites — pure churn that buries future real findings — so it is left as a reviewed,
non-enforced gate.)

## deadcode (35) — all false positives for this architecture
- `sdk/*` (Client.Run, WithTenant, parseApprovals, …): a **public library API**
  consumed by external programs, unreachable from this module's `main` by design.
- `peer.New`, `shell.New`: exported constructors; production uses the
  `peer.NewWithTenants` / `shell.NewWithWarden` variants — kept for API parity.
- sigv4 `canonicalQuery`/`awsURIEncode`/`sha256Hex`/`collapseSpaces`, jsonschemagen
  `readAll`, and the `Valid*`/`IsKnown`/`Timestamp` helpers: used by **tests**, which
  `deadcode` excludes from its root set.
Removing any would break the public API, the plugins, or the tests. Nothing to delete.

## shadow (~40) — all idiomatic, no error-masking
Every hit is `if err := f(); err != nil { … }` (or `x, err := …` in a nested block)
where the outer `err` was already handled before the block and the inner shadowed
`err` is consumed inside its own scope with an explicit `return`. The one that
shadows a *named return* (`journal.go:372` in `Restore`) returns explicitly inside
the block, so no stale-named-return bug. Idiomatic; not churned.

## gosec (~60) — all false-positive or by-design within the threat model
Threat model: localhost-only daemon (token-auth control plane), `agt` CLI run by a
**trusted operator** on their own machine, subprocesses launched only from
operator/config-controlled commands (the warden is the sandbox boundary).
- **G115** (uint64→byte in ulid.go, int→uint in cadence.go): intentional bit-masking
  during ULID/time encoding — the narrowing takes the low bits on purpose. FP.
- **G101** (creds.go/aws.go/vertex auth/main.go, all LOW confidence): matches on
  identifier/constant *names* containing "key"/"secret"/"token" (env var names, struct
  fields, token-type constants) — no literal secret. FP.
- **G703** (path traversal across `cmd/agt/*`): file paths supplied by the operator to
  a local CLI (backup output, plan/journal files). The operator already owns the
  filesystem; not a privilege boundary. By design.
- **G704** (SSRF): discord.go uses the fixed Discord API host (FP);
  skill_registry_remote.go fetches an **operator-specified** registry URL from the
  `agt` CLI — operator-driven, not the daemon serving untrusted input. (The daemon's
  agent-initiated fetches go through `netguard`, which blocks link-local/metadata —
  see netguard tests.) Acceptable.
- **G118** (Background ctx in goroutine): every site is the canonical graceful-shutdown
  pattern — the goroutine waits on the request `ctx.Done()`, then creates a fresh
  `context.WithTimeout(context.Background(), …)` for `srv.Shutdown` **because** the
  request ctx is now cancelled and must not be reused for the grace window. FP.
- **G122** (Walk symlink TOCTOU in file.go list / backup.go): the file tool already
  root-scopes via `resolve()` + symlink refusal; `os.Root` (Go 1.24+) is a possible
  future defense-in-depth, not a present vuln.
- **G702 / G204** (subprocess from variable): mcpbridge, coding (git), acpagent,
  plugin pin, aws `credential_process`, warden — all launch operator/config-declared
  commands; warden is itself the sandboxing security control. By design.
- **G401** (sha1 in sso.go): the AWS SSO cache filename **must** be
  `sha1hex(startURL).json` to interoperate with the AWS CLI's on-disk token cache —
  hashing a public URL to name a file, no secret, collision-resistance irrelevant. FP.
No genuine exploitable vulnerability surfaced.

## Verification / gate
- `go vet ./...` 0, `staticcheck ./...` 0, `gitleaks detect` 0.
- Tree builds (`GOOS=linux` clean), full `go test ./...` exit 0, `go.mod`/`go.sum`
  unchanged. (No code changed in M487 — it is a verification/triage milestone; the
  only code/config changes were M485 and M486.)

## Bottom line
Every objective gate available offline has been run. Two correctness/security gates
(staticcheck, gitleaks) were driven to zero and are now enforceable; `go vet` was
already zero. The remaining tools produced only idiomatic, false-positive, or
by-design findings, each classified above. One real external advisory (2 Go stdlib
CVEs) is documented with a single-line remediation (toolchain ≥ go1.26.4) that cannot
be applied offline. This is the affirmative, objective closure of the hardening goal
that the manual defect-hunt arc could not provide on its own.
