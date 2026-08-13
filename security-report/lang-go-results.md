# AGEZT — Go Language Deep Scan (Phase 2, `sc-lang-go`)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` · **Branch:** `main`
**Scope:** ~50,250 LOC Go / 1,572 files — `kernel/`, `cmd/agezt`, `cmd/agt`, `internal/`, `sdk/go`, `plugins/`, `contract/`, `tools/`
**Excluded:** `node_modules/`, `frontend/dist/`, `.git/`, `*.exe`, `.dev-home/` (owner's live daemon home — never read)

## Executive summary

**The Go layer is in markedly better shape than the architecture map's divergence list implies.**
`go vet ./...` and `staticcheck ./...` are both **completely clean** across the whole tree, there is
**zero `InsecureSkipVerify`**, **zero non-constant-time secret comparison**, and the panic-containment
architecture the recent WF-001 commits established is a real, documented, consistently-applied pattern
(`safePoll` / `safeFire` / `fireOne` / `recoverConn`), not a set of one-off patches.

The systematic panic hunt I was asked to run **did not find a remaining unrecovered-panic DoS**.
Every candidate I chased — 63 goroutines lacking an inline `recover()`, 38 unchecked type assertions —
was refuted on inspection (details in §Refuted). That negative result is the single most important
output of this scan and is reported as such.

What I did find is a **file-permission inconsistency around the Config Center's secret store**, which is
the only finding with real confidentiality impact, plus a **broken race-detector gate on
`kernel/controlplane`** that undermines the WF-001 regression control itself.

| ID | Title | Severity | Confidence |
|---|---|---|---|
| GO-001 | Config Center persists `RatingSecret` plaintext at `0644`, and the base dir's mode is boot-order dependent | **MEDIUM** | 88 |
| GO-002 | WF-001 regression control is race-red; `go test -race ./kernel/controlplane/` fails | **LOW** | 97 |
| GO-003 | TOCTOU between symlink guard and read in the `file` tool's walk | **LOW** | 70 |
| GO-004 | Rollback restores an unmasked `os.FileMode` (setuid/setgid/sticky reachable) | **LOW** | 72 |
| GO-005 | Inbound-email TLS sets no explicit `MinVersion` | **INFO** | 90 |
| GO-006 | `profileView` discards both JSON errors — latent nil-map write | **INFO** | 85 |

---

## Findings

### GO-001 — Config Center writes secret-rated values world-readable, in a base dir whose mode depends on boot order

- **Severity:** MEDIUM · **Confidence:** 88 · **CWE-276** (Incorrect Default Permissions), **CWE-732**
- **Category:** 8 — File Operations (SC-GO-153, SC-GO-163, SC-GO-170)

**Locations (all read directly):**

- `kernel/configcenter/center.go:385` — `return os.WriteFile(filename, data, 0644)`
- `kernel/configcenter/center.go:375` — `os.MkdirAll(c.config.Dir, 0755)` — **return value discarded**
- `kernel/configcenter/audit.go:113` — `f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)`
- `kernel/configcenter/audit.go:108` — `os.MkdirAll(a.dir, 0o755)`

**Why the data is sensitive.** `persistEntry` marshals the whole `ConfigEntry`, and that struct carries
the plaintext value alongside an explicit sensitivity rating:

```go
// kernel/configcenter/types.go:16
Value string `json:"value"`
// kernel/configcenter/types.go:19
Rating Rating `json:"rating,omitempty"`
```

`RatingSecret` is a real, used rating — `kernel/configcenter/config.go` maps it to `PolicyDeny` in
`AccessPolicies`, and `kernel/configcenter/types.go:310` filters on `entry.Rating == RatingSecret`.
`VaultBacked` (`types.go:36`) is *optional*: an entry rated secret but not vault-backed has its
cleartext `Value` written to `<baseDir>/configcenter/<key>.json` at mode `0644`.

**Why the 0700 base dir does not save it.** `internal/paths/paths.go:22-23` is explicit:

> `The directory is NOT created here; subsystems (journal, state, controlplane) create their own subdirs on first use.`

Go's `os.MkdirAll` documents that `perm` is used for **all** directories it creates, including parents.
The subsystems that create base-dir subtrees are split between two modes:

| Mode | Call site |
|---|---|
| `0o700` | `kernel/auth/tokenfile.go:30`, `kernel/agentgw/secret.go:50`, `kernel/artifact/artifact.go:52`, `kernel/artifact/index.go:60`, `kernel/datalake/datalake.go:116` |
| **`0o755`** | `kernel/controlplane/server.go:433`, `kernel/state/state.go:62`, `kernel/jsonstore/jsonstore.go:54`, `kernel/seat/store.go:49`, `kernel/tenant/tenant.go:179` |

On a fresh install, whichever subsystem writes first fixes `~/.agezt`'s mode. The control plane
(`server.go:433`, `0o755`) and `state`/`jsonstore` (`0o755`) are core boot-path components, so the
realistic outcome is a **`0755` base directory** — i.e. the `0644` secret files are genuinely
world-readable to any local user, not incidentally contained.

**Contrast — the vault gets this right.** `kernel/creds/creds.go:210-216` writes `creds.json` via
`atomicfile.WriteFile(..., 0o600)` and *re-applies* the mode after rename. Two secret stores in one
daemon, two different postures.

**Trigger condition.** Any operator or agent call that stores a secret-rated config value —
`POST /api/config/set` on the default-on console, or the `config` tool's `op=set` (`CapConfigWrite`,
`LevelAllow` by default) — lands cleartext at `0644`. No attacker action is needed to *create* the
exposure; a second local user (or any process not running as the operator) reads it.

**Why this is not a false positive.** I read all four lines. The struct field is plaintext `string`, the
rating enum includes `RatingSecret`, and `paths.go` disclaims base-dir creation in its own doc comment.
This is not the deliberate default-allow *capability* posture — it is a file mode, and the repo's own
credential vault demonstrates the intended standard.

**Caveat on Windows** (the owner's platform): Unix mode bits are ignored by `os.WriteFile`, so on Windows
protection is NTFS-ACL inheritance only and this finding is a latent portability/deployment issue rather
than an immediate local read. On Linux — the CI runners and `install.sh` systemd target — it is live.

**Remediation.**
1. `center.go:385` → `0o600`; `audit.go:113` → `0o600`; both `MkdirAll` → `0o700`.
2. **Check the `MkdirAll` error at `center.go:375`** — silently continuing means the subsequent
   `WriteFile` error is the first signal.
3. Create the base dir once, explicitly, at `0o700` during boot before any subsystem touches it, and
   normalise the ten `MkdirAll` call sites onto one constant so the mode stops depending on ordering.

---

### GO-002 — The WF-001 regression control fails under `-race`

- **Severity:** LOW · **Confidence:** 97 · **CWE-362** (Race Condition)
- **Category:** 11 — Concurrency (SC-GO-212, SC-GO-366)

**Location:** `kernel/controlplane/workflow_test.go:39` (write) and `:81` (read)

```go
// workflow_test.go:32
type panickingTool struct{ calls int }

// workflow_test.go:38-40
func (t *panickingTool) Invoke(_ context.Context, _ json.RawMessage) (agent.Result, error) {
	t.calls++          // written by the DETACHED run goroutine
	panic("node exploded")
}

// workflow_test.go:81
for tool.calls == 0 && time.Now().Before(deadline) {   // read by the TEST goroutine
```

`t.calls` is written from the detached workflow goroutine and polled from the test goroutine with no
synchronisation. Verified by running the test in isolation:

```
WARNING: DATA RACE
Write at 0x00c0003172a8 by goroutine 19:
  ...controlplane_test.(*panickingTool).Invoke()  workflow_test.go:39
Previous read at 0x00c0003172a8 by goroutine 9:
  ...TestWorkflow_AsyncRunPanicDoesNotKillTheDaemon()  workflow_test.go:81
--- FAIL: TestWorkflow_AsyncRunPanicDoesNotKillTheDaemon (0.08s)
FAIL	github.com/agezt/agezt/kernel/controlplane	0.254s
```

**This is a test-fixture race, not a production race.** I explicitly checked: the racing addresses are
the test double's field, and the production path (`kernel/controlplane/workflow.go:703` detached
goroutine → `workflow.go:332` `recover()`) behaves correctly. The daemon-survival assertion itself
passes; only the race detector fails the run.

**Why it still matters.** This test *is* the control that proves the WF-001 panic firewall stays fixed.
Per this repo's own history a test-only symbol can be a control rather than dead code — that applies
here. Today `GOMAXPROCS=3 go test -race ./kernel/controlplane/` is **RED**, which means (a) the race
gate for the entire `controlplane` package is unusable, and (b) the WF-001 guard is the specific thing
whose CI signal is broken. Given the repo's documented pattern of silently-red gates going unnoticed for
weeks, this is worth fixing rather than tolerating.

`wireEchoTool.last` (`workflow_test.go:27`) is the same unsynchronised pattern and will race the moment
its test polls concurrently.

**Remediation.** Make the counter `atomic.Int64` (or guard with a mutex) in `panickingTool` and
`wireEchoTool`, then confirm `go test -race ./kernel/controlplane/` is green.

---

### GO-003 — TOCTOU between the symlink guard and the read in the `file` tool's search walk

- **Severity:** LOW · **Confidence:** 70 · **CWE-367** (TOCTOU)
- **Category:** 8 — File Operations (SC-GO-160)

**Location:** `plugins/tools/file/file.go:602` (check) → `file.go:612` (use)

```go
if t.entryEscapesRoot(p, d) {
    return nil // skip a symlink whose target leaves the workspace
}
...
data, err := os.ReadFile(p)
```

The M427 guard **is present and correct** — my first hypothesis (that the guard was missing) was
refuted. The residual issue is the window between `entryEscapesRoot`'s `Lstat` and the `os.ReadFile`:
an entity that can replace `p` with a symlink inside that window reads a file outside the workspace root.

**Trigger condition.** Requires a writer racing the walk inside the workspace. The agent itself holds
`file` write access to that workspace by default, so it is self-reachable in principle, but the agent
already has legitimate read access to everything under the root — the escalation is only for paths
*outside* it, and it requires winning a narrow race. Hence LOW, and I flag the exploitability as
**partially unverified**: I did not build a working race harness.

Same pattern, lower reachability: `cmd/agt/backup.go:318`, `kernel/market/publish.go:121`,
`plugins/tools/codeexec/daytona.go:176`, `cmd/agt/skill_md.go:66`.

**Remediation.** Go 1.24+ ships `os.Root` (this repo builds on **go 1.26.5**). Opening the workspace
once as an `os.Root` and using `root.Open`/`root.Stat` makes containment kernel-enforced and closes the
window structurally.

---

### GO-004 — Rollback restores an unmasked `os.FileMode`

- **Severity:** LOW · **Confidence:** 72 · **CWE-732**
- **Category:** 8 — File Operations

**Locations:** `kernel/webui/rollback.go:225-227`, `cmd/agt/rollback.go:311-313`

```go
perm := os.FileMode(0o644)
if n := rollbackIntNumber(before["mode_perm"]); n > 0 {
    perm = os.FileMode(n)
}
```

`rollbackIntNumber` (`rollback.go:313-327`) accepts `int`, `int64`, `float64`, `json.Number` from the
stored rollback record and returns a bare `int`. It is converted to `os.FileMode` with **no `& 0o777`
mask**, so values above `0o777` set Go's high mode bits — `os.ModeSetuid` (`1<<23`), `os.ModeSetgid`,
`os.ModeSticky` — on the restored file.

**Why not higher.** The catalog is written by the daemon recording its own before-state, and
`/api/rollback/apply` is operator-gated, so this is not directly attacker-driven; it is a
defence-in-depth gap that turns a corrupted or agent-influenced catalog entry into a setuid write.
I did **not** trace every writer of `mode_perm` to prove an agent can set it, so treat the reachability
as unverified — the missing mask itself is confirmed by reading both files.

**Remediation.** `perm = os.FileMode(n) & 0o777`.

---

### GO-005 — Inbound email TLS sets no explicit `MinVersion` (INFO)

`plugins/channels/email/inbound.go:256` and `:274` are the **only two** `tls.Config` literals in the
non-test tree:

```go
conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", c.inboxAddr, &tls.Config{ServerName: host})
tconn := tls.Client(conn, &tls.Config{ServerName: host})
```

Certificate verification is **on** (`ServerName` set, `InsecureSkipVerify` absent), so this is not a
verification bypass. Go's client default `MinVersion` is TLS 1.2, so the effective posture is already
acceptable; setting `MinVersion: tls.VersionTLS12` explicitly just makes it immune to a future default
change. INFO only.

---

### GO-006 — `profileView` discards both JSON errors; latent nil-map write (INFO)

`kernel/controlplane/roster.go:175-182`:

```go
func profileView(p roster.Profile) map[string]any {
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["kind"] = p.Kind()      // panics if m is nil
	...
```

If `json.Marshal` ever failed, `b` would be nil, `Unmarshal` would leave `m` nil, and `m["kind"] = ...`
would panic with *assignment to entry in nil map* — inside a control-plane handler.

**Refuted as exploitable.** I read the whole `roster.Profile` struct (`kernel/roster/roster.go:41-120`):
every field is `string`, `int64`, `bool`, `[]string`, `*struct`, `map[string]string`, or a slice of
plain structs. `encoding/json` cannot fail on any of these (no channels, funcs, complex, or cyclic
pointers). The panic is unreachable today. Reported as INFO because it is one struct-field addition
(e.g. a `func` or `chan` field) away from becoming a live control-plane DoS, and because the
control plane's `recoverConn` (`server.go:606`) would convert it to a generic error rather than surface
the bug.

---

## Refuted candidates (false positives killed in verification)

Recording these is deliberate — they are the bulk of the scan's value.

| Candidate | Why it is not a finding |
|---|---|
| **63 goroutines with no inline `recover()`** | Sampled and traced the reachable ones. `plugins/channels/*/…` (15 internet-facing listeners), `kernel/agentgw/gateway.go:212`, `kernel/runtime/runtime.go:941`, `kernel/httpserver/listener.go:58` are all `srv.Serve` / `ListenAndServe` wrappers or `<-ctx.Done()` shutdown watchers. **`net/http` recovers handler panics per-connection**, so a malformed request yields a 500 + dropped connection, not process death. The genuinely bare dispatch paths are already wrapped: `kernel/pulse/engine.go:436` (`safePoll`), `kernel/standing/runner.go:69,210` (`safeFire`), `kernel/cadence/cadence.go:1387` (`fireOne`), `kernel/controlplane/workflow.go:332`, `kernel/selfrepair/selfrepair.go:217,978`, `kernel/agent/run_tools.go:306` (per-tool), `cmd/agezt/main.go:2791`. 26 `recover()` sites, deliberately placed and commented. |
| **38 unchecked type assertions** | `kernel/pulse/engine.go:328` (`ts_unix_ms`), `kernel/controlplane/sandbox.go:91` (`name`), `kernel/controlplane/roster.go:1027,1040,1050` (`seq`), `plugins/tools/mcptool/tool.go:190` (`name`) all assert on maps **constructed literally a few lines above** with statically-typed struct fields — structurally guaranteed. `kernel/controlplane/roster.go:272,280` survives too: `profileView` always returns `map[string]any`, and `Slug string \`json:"slug"\`` has **no `omitempty`**, so the key is always present and always a string after the JSON round-trip. `kernel/governor/cache.go:94-119` asserts on its own `container/list` values. The remaining ~22 are in `cmd/agt` (CLI), where a panic kills a short-lived CLI process, not the daemon. |
| **gosec G101 ×21 "hardcoded credentials"** | Every hit is an **env-var name constant** — `SecretEnvLocal = "AGEZT_EXEC_SECRET_ENV_LOCAL"`, `RemoteSecretPolicyEnv = "AGEZT_EXEC_REMOTE_SECRET_POLICY"`, etc. No secret material. |
| **gosec G118 ×16 "goroutine uses context.Background"** | All are graceful-shutdown watchers: `go func(){ <-ctx.Done(); shutCtx, _ := context.WithTimeout(context.Background(), 5*time.Second); srv.Shutdown(shutCtx) }()`. Using `Background` there is **correct** — the request context is already cancelled and cannot drive a graceful drain. |
| **gosec G404 ×2 `math/rand`** | `plugins/providers/internal/retry/retry.go:206` and `kernel/governor/governor.go:631` — both are **retry-backoff jitter**, a non-security use. Correct choice. |
| **gosec G110 decompression bomb, `cmd/agt/backup.go:507`** | Backup archives are **secret-free by construction**: `backupIncludeDirs = []string{"journal", "catalog"}` (`backup.go:43`), and `backup.go:38-42` documents that `creds.json` and `runtime/control.token` live outside those subtrees. The `0644` restore mode (`backup.go:503`) therefore cannot downgrade the vault — my initial hypothesis, refuted. Extraction is guarded by `isAllowedBackupPath` (`:519-530`), a second lexical prefix check (`:496`), and `O_EXCL` (`:503`). |
| **gosec G120 ×2 unbounded form parsing** | `kernel/webui/transcribe.go:35` and `kernel/openaiapi/openaiapi.go:235` both call `ParseMultipartForm(audioMaxBytes)` **after** a `MaxBytesReader` wrap. Bounded. |
| **gosec G115 ×18 integer overflow** | `kernel/ulid/ulid.go:67-72` is deliberate big-endian byte packing; `plugins/channels/nostr/nip19.go:92-126` is standard bech32 5↔8-bit regrouping. Both intentional truncation. (The two `os.FileMode` conversions are split out as GO-004.) |
| **gosec G103 ×6 `unsafe`** | All are Windows syscall marshalling — `kernel/pulse/diskusage_windows.go:27-30`, `kernel/creds/machineid_windows.go:39`, `plugins/tools/file/nofollow_windows.go:46`. Idiomatic `uintptr(unsafe.Pointer(&buf[0]))` for `syscall.Syscall`; no pointer arithmetic, no type-punning, no `reflect` field access. |
| **gosec G602 ×12 slice index** | All in `cmd/agt/main.go:162-301` CLI flag parsing, each guarded by the surrounding `i+1 < len(args)` idiom; CLI-only regardless. |
| **gosec G702/G704 command-injection / SSRF taint** | `cmd/agezt/httpsurfaces.go:259-263` is the boot "open browser" on the daemon's own console URL. The rest (`cmd/agt/peers.go`, `ha.go`, `plugin_registry.go`, `skill_registry_remote.go`) are **CLI** commands against operator-supplied endpoints — by design, and outside the daemon's trust boundary. |
| **`plugins/tools/file/file.go` walk symlink escape** | The M427 guard `t.entryEscapesRoot(p, d)` **is present** at `file.go:602`. Only the residual TOCTOU window remains (GO-003). |
| **Secret comparison with `==`** | Swept the non-test tree; no secret/token/HMAC compared with `==`. Consistent with the architecture map's constant-time claim (`kernel/auth/token.go:72`, `kernel/webui/session.go:224`, `kernel/tenant/tenant.go:222`). |
| **`gofmt` reporting ~all files unformatted** | Working-tree artefact: `core.autocrlf=true`, files are CRLF on disk. Extracting `HEAD` blobs to a temp dir and running `gofmt -l` returns **empty** — the index content is clean, and CI is therefore green. Not a finding. |

---

## Checklist coverage

| # | Category | Result |
|---|---|---|
| 1 | Input Validation & Sanitization | **Clean.** Webhook inbound (`plugins/channels/webhook/webhook.go:183-220`) is exemplary: `io.LimitReader`, signature verified *before* parse, replay window compared in integer ms with an explicitly-reasoned overflow guard, dedup. |
| 2 | Authentication & Session Management | **Clean (Go layer).** Constant-time compares throughout; cookie `HttpOnly`+`SameSite=Strict`. gosec G124 ×2 on `session.go:235,288` refuted — `Secure` is set conditionally from `r.TLS`/`X-Forwarded-Proto` by design. Default-password posture is `sc-recon`'s DIVERGENCE 4, not a Go-layer defect. |
| 3 | Authorization & Access Control | Out of scope for lang-go (Edict posture = owner decision + Phase-1 divergences). |
| 4 | Cryptography | **Clean.** 0 `InsecureSkipVerify`; AES-256-GCM with per-save salt+nonce; `hmac.Equal` used. SHA-1 hits are protocol-mandated (WeCom/OneBot/SMS signatures, AWS SSO cache-key naming at `kernel/creds/sso.go:80`), not security primitives. `math/rand` is jitter-only. 1 INFO (GO-005). |
| 5 | Error Handling & Logging | **1 finding** (GO-001's discarded `MkdirAll`). 103 gosec G104s reviewed — the rest are best-effort `os.Remove`/`Close` with explanatory comments. |
| 6 | Data Protection & Privacy | **1 finding** (GO-001). |
| 7 | SQL/NoSQL/ORM | **N/A — no `database/sql`, no driver, no query strings in the tree.** No SQL injection surface exists. |
| 8 | File Operations | **3 findings** (GO-001, GO-003, GO-004). Traversal/symlink/zip-slip guards are otherwise strong and load-bearing. |
| 9 | Network & HTTP Security | **Clean.** Timeouts set on every server I checked (`kernel/httpserver/listener.go:12-35`; each channel's `newHTTPServer` sets `ReadHeaderTimeout`/`ReadTimeout`/`IdleTimeout`, with `WriteTimeout` intentionally unset for SSE and documented as such). Body caps declarative. |
| 10 | Serialization & Deserialization | **Clean.** No `encoding/gob`, no YAML, no XML, no `json.Unmarshal` into bare `any` with type dispatch outside a fuzz harness. All decoding targets concrete structs. gosec G117 (`chatgptauth.go:117` marshals `AccessToken`) is intentional token persistence, not a wire leak. |
| 11 | Concurrency & Race Conditions | **1 finding** (GO-002). `kernel/agentgw` and `kernel/configcenter` pass `-race` clean. |
| 12 | Dependency & Supply Chain | **Clean (Go side).** 5 direct deps, `go.sum` committed, no `replace` directives, no `+incompatible`. Deferred to `sc-supply-chain`. |
| 13 | Configuration & Secrets Management | **1 finding** (GO-001). No hardcoded secrets in Go source — all 21 G101 hits are env-var names. |
| 14 | Memory Safety | **Clean.** 6 `unsafe` uses, all Windows syscall marshalling. No pointer arithmetic, no `reflect` unexported-field access, no `//go:linkname`, no cgo. |
| 15 | Go-Specific Patterns | **Clean.** No `text/template` **or** `html/template` anywhere. Type assertions verified (see Refuted). `go vet` clean. |
| 16 | Framework-Specific | **N/A** — stdlib `net/http` + `ServeMux` only; no Gin/Echo/Fiber. |
| 17 | API Security | Deferred to `sc-api`; webhook signature verification confirmed present. |
| 18 | Testing & CI/CD | **1 finding** (GO-002). `staticcheck` and `go vet` are clean and worth enforcing as gates. |
| 19 | Logging & Monitoring | **Clean.** No `net/http/pprof` import in the non-test tree. Structured `slog` throughout. |
| 20 | Third-Party Integration | Netguard bypass inventory is `sc-recon` §5.8; no new Go-layer defect found. |

---

## Tooling output

Exactly what I ran and what it returned. `GOMAXPROCS=3` on every Go command (32-core host, capped per repo convention).

| Command | Result |
|---|---|
| `GOMAXPROCS=3 go vet ./...` | **Exit 0, zero diagnostics.** Covers copylocks / printf / lostcancel, which are vet-only. |
| `GOMAXPROCS=3 staticcheck ./...` | **Exit 0, zero findings** (0-byte output) across the whole module. |
| `GOMAXPROCS=3 gosec -quiet ./...` | Exit 0. **419 issues over 629 files / 194,462 lines.** Full histogram below; triaged in Findings + Refuted. |
| `git ls-files '*.go' \| xargs gofmt -l` | Lists ~all files — **CRLF artefact**, `core.autocrlf=true`. Re-checked by extracting `HEAD` blobs to a temp dir: `gofmt -l` returns empty ⇒ index content is clean. |
| `GOMAXPROCS=3 go test -race -count=1 ./kernel/controlplane/ ./kernel/agentgw/ ./kernel/configcenter/` | `agentgw` **ok** (3.07s), `configcenter` **ok** (1.42s), `controlplane` **FAIL** — data race, isolated below. |
| `GOMAXPROCS=3 go test -race -count=1 -run TestWorkflow_AsyncRunPanicDoesNotKillTheDaemon ./kernel/controlplane/` | **FAIL** — `WARNING: DATA RACE`, write `workflow_test.go:39` vs read `workflow_test.go:81`. Full trace quoted in GO-002. |
| `govulncheck` | Installed (`/c/Users/ersin/go/bin/govulncheck`) but **not run** — dependency CVE scanning is `sc-supply-chain`'s scope, not lang-go's. Flagging availability so that phase can use it. |

`gosec` rule histogram (all 419):

```
103  G104 (CWE-703)  Errors unhandled                                   LOW
 81  G304 (CWE-22)   Potential file inclusion via variable              MEDIUM
 50  G703 (CWE-22)   Path traversal via taint analysis                  HIGH
 37  G301 (CWE-276)  Directory permissions > 0750                       MEDIUM
 21  G101 (CWE-798)  Potential hardcoded credentials                    HIGH   -> all env-var NAMES
 16  G118 (CWE-400)  Goroutine uses context.Background                  HIGH   -> all shutdown watchers
 13  G306 (CWE-276)  WriteFile permissions > 0600                       MEDIUM -> GO-001
 12  G602 (CWE-118)  Slice index out of range                           LOW    -> CLI flag parsing
 12  G204 (CWE-78)   Subprocess launched with variable                  MEDIUM
  9  G704 (CWE-918)  SSRF via taint analysis                            HIGH   -> CLI / boot browser
  6  G302 (CWE-276)  File permissions > 0600                            MEDIUM
  6  G204 (CWE-78)   Subprocess w/ potential tainted input              MEDIUM
  6  G122 (CWE-367)  Walk/WalkDir TOCTOU                                HIGH   -> GO-003
  6  G115 (CWE-190)  uint64 -> byte                                     HIGH   -> ulid packing
  6  G103 (CWE-242)  Unsafe calls                                       LOW    -> Windows syscalls
  5  G702 (CWE-78)   Command injection via taint analysis               HIGH   -> CLI / boot browser
  4  G705 (CWE-79)   XSS via taint analysis                             MEDIUM
  4  G505 (CWE-327)  Blocklisted import crypto/sha1                     MEDIUM -> protocol-mandated
  3  G115 (CWE-190)  uint32 -> byte / int -> uint                       HIGH
  2  G404 (CWE-338)  Weak RNG                                           HIGH   -> backoff jitter
  2  G401 (CWE-328)  Weak crypto primitive (sha1)                       MEDIUM
  2  G124 (CWE-614)  Cookie attribute                                   MEDIUM -> conditional Secure by design
  2  G120 (CWE-400)  Unbounded form parsing                             MEDIUM -> MaxBytesReader-wrapped
  2  G115 (CWE-190)  rune -> byte / int -> uint32                       HIGH   -> GO-004
  1  G117 (CWE-499)  Marshaled "AccessToken"                            MEDIUM -> intentional persistence
  1  G110 (CWE-409)  Decompression bomb                                 MEDIUM -> secret-free archive
```

G304 (81) and G703 (50) are the dynamic-path families; both are dominated by the file-tool and
File-Manager paths whose containment guards `sc-recon` §4.3(b) already enumerated and which I
re-confirmed present at `plugins/tools/file/file.go:602` and `kernel/webui/files_route.go`. I did not
re-audit all 131 individually — that is the path-traversal skill's scope, and no new Go-idiomatic defect
appeared in the sample I read.

---

## Notes for the final report

1. **The headline is a negative result.** The remaining-panic hunt this phase was commissioned to run
   came back empty. The WF-001 work appears to have been done systematically rather than
   symptomatically: the codebase has a named, documented, mirrored containment pattern across `pulse`,
   `standing`, `cadence`, `workflow`, `selfrepair`, and the control plane, plus per-connection recovery
   in `net/http` and `recoverConn`. Say so explicitly — it is the strongest positive signal in the
   Go layer and it contradicts the pessimistic prior.
2. **`go vet` and `staticcheck` are both clean.** These are cheap, currently-passing gates. Given this
   repo's documented history of gates silently going red for weeks, recommend wiring both into CI as
   required checks *while they are green*.
3. **GO-002 is small but strategically annoying:** the one package whose race gate is broken is the one
   holding the WF-001 regression control. Two-line fix.
4. **GO-001's base-dir mode issue is worth raising beyond the Config Center.** Ten `MkdirAll` call sites
   split 5/5 between `0700` and `0755` for the same base directory means the daemon's home permissions
   are a function of boot ordering. That deserves a single owning constant regardless of the
   Config Center fix.
