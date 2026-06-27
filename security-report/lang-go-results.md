# Go Language Deep Scan — Results (`sc-lang-go`)

> Repo: `D:\Codebox\PROJECTS\AGEZT`. Scope: Go-idiom weakness classes (concurrency, error
> handling, unsafe/reflect, integer conversion, resource lifecycle, randomness, file perms,
> defer/panic). Injection/auth/SSRF are covered by other scanners and are excluded here except
> where a Go idiom is the root cause. `*_test.go` treated as context, not findings. The
> `.worktrees\rebased-main\` tree is a duplicate working copy and was excluded from findings.

## Severity summary

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 2 |
| Low      | 3 |
| Info     | 3 |

**Overall posture: strong.** The Go codebase is unusually well-hardened against language-idiom
bugs. `InsecureSkipVerify` appears nowhere. All real LLM provider response reads go through a
bounded `httpread.All` / `io.LimitReader`. Every public HTTP listener (web UI, REST, OpenAI-compat,
agentgw, channel webhooks) sets slow-loris timeouts (`ReadHeaderTimeout`/`ReadTimeout`/`IdleTimeout`),
with a deliberate, tested decision to leave `WriteTimeout` unset for SSE/streaming. The agentgw
rate-limit map is correctly `rlMu`-guarded with a bounded entry table (CWE-770). Production-code type
assertions use the two-value `,ok` form; bare panicking assertions are confined to tests. `recover()`
is used deliberately for daemon-resilience panic isolation in watcher/loop goroutines. `math/rand` is
used only for retry-backoff jitter (non-security); all security randomness uses `crypto/rand`.
`unsafe.Pointer` appears only in OS-syscall bridge files (warden Prlimit, Windows disk/machine-id).
File permissions are correct throughout (0o600 sensitive, 0o700 sandbox dirs, 0o644 general).

---

## MEDIUM

### [MEDIUM] Data race on process-global channel-registry maps (boot-window concurrent map read/write → daemon crash)

- **Category:** 4 / 19 — Race Conditions / Slice-Map Concurrent Access (SC-GO-218, SC-GO-231)
- **Location:** `kernel/channel/registry.go:46`, `:49`, `:52-58`, `:62-65`, `:70`, `:74-80`, `:83`, `:97`, `:100-106`, `:110`
- **Pattern Matched:** Package-global `map` (`registry`, `live`, `liveInstances`) read and written from multiple goroutines with **no `sync.Mutex`/`sync.RWMutex`**.
- **Description:** Three process-wide maps are mutated and read without any synchronization:
  - `registry` — written by `RegisterManifest` (`:49`), iterated by `Manifests` (`:54`), read by `LookupManifest` (`:63`).
  - `live` — replaced wholesale by `SetLive` (`:79`), read by `IsLive` (`:83`).
  - `liveInstances` — replaced by `SetLiveInstances` (`:105`), read by `IsLiveInstance` (`:110`).

  The writers run during daemon boot: `builtinchannels.RegisterAll()` → `RegisterManifest` (`cmd/agezt/main.go:1933`) and `channel.SetLive`/`SetLiveInstances` (`cmd/agezt/main.go:1827-1828`). The control-plane listener, however, has **already begun serving** at `srv.Start(ctx)` (`cmd/agezt/main.go:1368`), i.e. *before* those writes. The readers (`Manifests`, `IsLive`, `IsLiveInstance`, `LookupManifest`) are reached from control-plane handlers `kernel/controlplane/channels.go:90-119` and `channel_accounts.go:30,130`, which serve concurrent RPC/web-UI connections.
- **Exploitability:** A `/api/channel/list` (or channel-accounts) request that lands in the boot window — between the control-plane starting to serve and `RegisterAll`/`SetLive` completing — races a map write against a `range`/index read. Go's runtime detects `concurrent map read and map write` and **fatally aborts the whole process** (unrecoverable, bypasses `recover()`). This is a remote-trigger-able DoS for any client that can reach the control plane during startup (the web UI auto-polls the channel list on load). The window is short and timing-dependent, but the fault is a hard crash, not corruption. It would also fire under `go test -race`.
- **Remediation:** Add a package-level `sync.RWMutex` to `kernel/channel/registry.go` guarding all three maps: `RLock` in `Manifests`/`LookupManifest`/`IsLive`/`IsLiveInstance`, `Lock` in `RegisterManifest`/`SetLive`/`SetLiveInstances`. (Replacing the map reference in `SetLive`/`SetLiveInstances` is itself a racy write of the map *variable* against a concurrent read, so the lock is required even though those setters swap rather than mutate in place.)
- **Reference:** CWE-362; https://go.dev/ref/mem ; runtime `concurrent map read and map write`.
- **Confidence:** High (writers and readers, and the boot-time ordering relative to the listener, all confirmed by reading the call sites).

### [MEDIUM] Unbounded `io.ReadAll` on response body in `retry.ReadBody` (memory-exhaustion DoS if reached)

- **Category:** 11 / 14 — Deserialization / Memory Safety (SC-GO-177, SC-GO-281)
- **Location:** `plugins/providers/internal/retry/retry.go:259` and `:262`
- **Pattern Matched:** `io.ReadAll(resp.Body)` with **no `io.LimitReader`** wrapping.
- **Description:** `ReadBody` reads an entire HTTP response into memory with no cap on both the error path (`:259`) and the success path (`:262`). Every other provider response read in the tree uses the bounded `httpread.All(..., httpread.DefaultMaxResponseBytes)` helper, so this function is the lone unbounded reader. A malicious or compromised upstream provider/proxy endpoint (operator-configured, so semi-trusted) returning a multi-GB body would OOM the daemon.
- **Exploitability:** Lower than the registry race because the caller must be an operator-configured provider endpoint, and a grep of the current provider call paths shows the live providers route through `httpread.All` rather than `ReadBody` — so this is a latent/edge helper, not the hot path today. Risk is realized if any future caller wires `ReadBody` onto an attacker-influenceable endpoint.
- **Remediation:** Wrap both reads in `io.LimitReader(resp.Body, httpread.DefaultMaxResponseBytes+1)` (or call `httpread.All`) and surface a "response too large" error, matching the rest of the providers package.
- **Reference:** CWE-770; https://pkg.go.dev/io#LimitReader
- **Confidence:** High on the unbounded read; Medium on reachability (no current hot-path caller observed).

---

## LOW

### [LOW] Long-lived daemon goroutines pinned to `context.Background()` (shutdown / cancellation hygiene)

- **Category:** 14 — context.Context Misuse (SC-GO-219, SC-GO-301)
- **Location:** `kernel/runtime/runtime.go:892` (`agentGW.Listen(context.Background())`); `kernel/controlplane/roster.go:~2099→~2579` (`runAgentWake` builds `runtime.WithAgentProfile(context.Background(), p)`)
- **Description:** A few background services and request-spawned runs use `context.Background()` rather than a cancellable parent. For the agentgw listener this means kernel `Halt()` does not propagate cancellation to that goroutine; for `runAgentWake` it means an operator-initiated wake cannot be cancelled by the originating request. These are mostly intentional "fire-and-forget, lives for the process / outlives the request" designs (an agent wake *should* survive the HTTP request that triggered it), so impact is limited to slightly less graceful shutdown and non-cancellable in-flight work — not a memory leak per se.
- **Remediation:** Where the work genuinely should outlive the request, document it; otherwise derive from a daemon-scoped cancellable context so `Halt()` can drain it. Low priority.
- **Reference:** CWE-400 / CWE-772.
- **Confidence:** Medium (the cancellation gap is real; whether it is a bug or a deliberate lifetime choice depends on intended semantics — leaning "by design" for the wake path).

### [LOW] Plugin→host callback uses `context.WithTimeout(context.Background(), …)` instead of request context

- **Category:** 14 — context.Context Misuse (SC-GO-301)
- **Location:** `kernel/plugin/host.go:918`
- **Description:** `handleCallback` bounds a host-tool invoke with `context.WithTimeout(context.Background(), p.cfg.InvokeTimeout)` rather than deriving from the inbound request context. A client/plugin disconnect won't cancel the in-flight tool early; it runs until completion or `InvokeTimeout`. There **is** a timeout, so this is bounded resource use, not an unbounded leak.
- **Remediation:** Derive the timeout context from the request/run context (`context.WithTimeout(r.Context(), p.cfg.InvokeTimeout)`) so disconnect cancels promptly.
- **Reference:** CWE-400.
- **Confidence:** High on the pattern; Low on impact (timeout caps the blast radius).

### [LOW] `ReadBody` discards the read error on the non-2xx path

- **Category:** 5 — Error Handling (SC-GO-110)
- **Location:** `plugins/providers/internal/retry/retry.go:259` (`body, _ := io.ReadAll(resp.Body)`)
- **Description:** The error from reading the error-response body is discarded with `_`. Combined with the unbounded read above, a truncated/failed read silently yields a partial `HTTPError.Body`. Not security-critical (it's an error-formatting path), but it is an ignored I/O error on a path that shapes an error returned upward.
- **Remediation:** At minimum bound the read (see MEDIUM above); optionally note read truncation in the `HTTPError`.
- **Reference:** CWE-391.
- **Confidence:** High.

---

## INFO (verified-clean / defensive observations)

### [INFO] `unsafe.Pointer` usage is confined to OS-syscall bridges — acceptable
- **Location:** `kernel/warden/warden_linux.go:129` (Prlimit), `kernel/pulse/diskusage_windows.go:27-30` (GetDiskFreeSpaceEx), `kernel/creds/machineid_windows.go:39` (RegQueryValueEx).
- **Note:** All three are the canonical, minimal `unsafe.Pointer` casts required to call platform syscalls; pointers do not escape, no pointer arithmetic, no `reflect`/`uintptr` round-tripping retained. Documented in-line (warden). No action. (SC-GO-276 reviewed, not violated.)

### [INFO] Randomness posture is correct
- **Note:** Only `math/rand` use in production is retry-backoff jitter (`plugins/providers/internal/retry/retry.go:207`) and `math/rand/v2` in `kernel/governor/governor.go` (load-spreading) — both non-security. Tokens/nonces/salts/OAuth-state use `crypto/rand` (per architecture §7/§9). No `math/rand` reaches a security-sensitive value. (SC-GO-066 / SC-GO-297 satisfied.)

### [INFO] HTTP-server slow-loris timeouts + body caps are applied uniformly
- **Note:** `newGuardedHTTPServer` (`cmd/agezt/main.go:4288`) wraps web UI / REST / OpenAI-compat with `ReadHeaderTimeout`+`IdleTimeout` (WriteTimeout deliberately unset for SSE, tested in `httpserver_test.go`). agentgw sets Read/Write timeouts + `MaxHeaderBytes 1<<20`. Every channel webhook server (`discord`, `webhook`, `whatsapp`, `feishu`, `zalo`, `wecom`, `dingtalk`, `whatsappgw`, …) sets `ReadHeaderTimeout`+`ReadTimeout`, with dedicated slowloris tests. Untrusted bodies are bounded (`MaxBytesReader`, `io.LimitReader`, multipart `audioMaxBytes`). (SC-GO-171 / SC-GO-321 satisfied.)

---

## Methodology / coverage notes
- Discovery via sink-pattern greps across `kernel/`, `cmd/`, `plugins/`, `internal/`, `sdk/go`, `tools/`: `io.ReadAll`, `InsecureSkipVerify`, `unsafe.`/`reflect.NewAt`/`//go:linkname`, `math/rand`, `recover()`, `os.OpenFile`/`WriteFile`/`MkdirAll`/`Chmod` perms, `http.Server{}`/timeouts, narrowing int conversions, type assertions, `go func(`.
- Concurrency breadth covered by two delegated read-only sub-scans (unguarded shared maps/slices; send-on-closed-channel / goroutine-leak / WaitGroup / deadlock / context). No send-on-closed-channel, WaitGroup-misuse, or lock-inversion deadlock found in production code; the channel-registry globals were the one concrete data race confirmed by reading call sites.
- Hotspots verified by direct read: `kernel/channel/registry.go`, `cmd/agezt/main.go` (boot ordering 1368/1827/1933 + `newGuardedHTTPServer`), `kernel/agentgw/gateway.go` (rate-limit mutex), `plugins/tools/codeexec/codeexec.go` (file perms), `kernel/webui/transcribe.go` (bounded multipart), `cmd/agt/plugin_registry.go` (0o755 is a verified executable, not a finding), `plugins/providers/internal/retry/retry.go`.
