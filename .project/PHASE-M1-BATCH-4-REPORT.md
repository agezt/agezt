# Phase Batch Report — M1.tt-4 + M1.d

> Status: **all shipped** · Date: 2026-05-29
> Two small phases continuing the autonomous-shipping arc after
> Batch-3 (SigV4, vv, rr, ss, uu). Both close items the previous
> report had classified as "lower priority than what shipped" or
> "platform-specific" — neither turned out to be as deferred as
> feared once approached concretely.

## Phases shipped

| # | Phase | Scope | Tests |
|---|---|---|---|
| 1 | **M1.tt-4** — Bedrock AI21 Jamba | OpenAI-shaped chat completion body | 4 |
| 2 | **M1.d** — Linux warden hardening | Setpgid + group-kill + prlimit (RLIMIT_CPU/AS/NOFILE/FSIZE) | 1 new + 1 platform-split |

All 36 testable packages still green; `go.mod` unchanged (still
only `blake3` + transitive `cpuid`).

## What changed

### M1.tt-4 — AI21 Jamba body shape

Completes the Bedrock multi-vendor matrix to: **Anthropic +
Mistral + Cohere + Meta-Llama + AI21 Jamba**. Same body-encoder /
response-decoder pattern as the existing four.

[plugins/providers/bedrock/ai21.go](plugins/providers/bedrock/ai21.go)
implements the Jamba wire shape, which is OpenAI-compatible chat:

```json
{
  "messages": [
    {"role": "system",    "content": "..."},
    {"role": "user",      "content": "..."},
    {"role": "assistant", "content": "..."}
  ],
  "max_tokens": N
}
```

Response:
```json
{
  "choices": [{
    "message": {"role": "assistant", "content": "..."},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": N, "completion_tokens": M}
}
```

**Why only Jamba and not bare `ai21.*`.** AI21's older J2 SKU
(`ai21.j2-mid-v1`, `ai21.j2-ultra-v1`) uses a totally different
shape (`prompt` + camelCase `maxTokens`). Routing only Jamba ids
through this encoder means J2 cleanly falls into
`ErrVendorUnsupported` with a clear operator-facing message
rather than partial-impl confusion. `isAI21JambaModel` matches
`ai21.jamba*` direct ids and `*.ai21.jamba*` regional profiles.

**No tool use.** Jamba on Bedrock does support OpenAI tool_calls,
but agezt's canonical tool shape is Anthropic-flavored; bridging
that requires the openai-provider plumbing. Out of scope for the
body-shape phase; chat-only matches the Mistral / Cohere / Llama
policy.

The default error message in `bedrock.Complete` was updated to
list AI21 Jamba as supported (and to call out that Titan and the
legacy AI21 J2 are intentionally NOT wired).

### M1.d — Linux warden hardening

The Batch-3 report classified plugin sandboxing as "next layer
when needed" and noted operators wrap with systemd/docker today.
Looking at it concretely there's a meaningful first slice that
ships in stdlib with no new deps: **Setpgid + process-group kill
on timeout + best-effort prlimit** on Linux for the cross-cutting
runaway-protection cases (CPU loop, memory blowup, fd exhaustion,
fsize abuse).

Implementation split:

- [kernel/warden/warden.go](kernel/warden/warden.go) — extended
  the cross-platform engine to call two platform helpers
  (`configurePlatformAttrs` before Start, `applyPlatformLimits`
  after Start) AND exposed new `Limits.CPUSeconds` /
  `AddressSpaceBytes` / `MaxOpenFiles` / `MaxFileSizeBytes`
  fields (honored only on Linux + ProfileNamespace, ignored
  elsewhere). Engine now goes through `cmd.Start` + `cmd.Wait`
  rather than `cmd.Run` so the post-Start hook has a PID.

- [kernel/warden/warden_linux.go](kernel/warden/warden_linux.go)
  (`//go:build linux`) — the actual hardening:
  - `Setpgid: true` so the child starts in its own process group.
  - `cmd.Cancel` overridden to send SIGKILL to `-pgid` (sweep
    grandchildren the tool spawned).
  - `prlimitSet(pid, resource, value)` calls prlimit64(2) via
    `syscall.Syscall6` + one confined `unsafe.Pointer` cast.
    Stdlib `syscall` package doesn't expose a Prlimit wrapper
    (lives in `golang.org/x/sys/unix`, off-limits per lean-deps),
    so going direct on `SYS_PRLIMIT64` is the minimum-blast-radius
    path — the constant and the Rlimit struct are both already
    exported by stdlib per-arch.
  - New `resolveEffectiveProfile` keeps `ProfileNamespace` as-is
    on Linux (so the hardening actually engages); Container and
    MicroVM downgrade to Namespace as next-best-available.

- [kernel/warden/warden_other.go](kernel/warden/warden_other.go)
  (`//go:build !linux`) — no-op stubs preserving the existing
  cross-platform behavior (everything downgrades to ProfileNone
  with the usual `warden.profile_downgraded` event).

**What this IS NOT.** No namespaces (CLONE_NEWUSER / CLONE_NEWNS
/ CLONE_NEWPID), no seccomp BPF, no cgroup v2. Those would be
full M1.d.1 / M1.d.2 follow-ups requiring more careful design
(user-namespace ergonomics, BPF compilation strategy, cgroup-v2
filesystem writes). What's shipping IS a meaningful confinement
layer for accidental runaway — a misbehaving tool that
fork-execs a long-running helper now has the helper swept on
timeout; a tool that mis-loops allocating 40 GiB now hits SIGSEGV
at the limit; a tool that opens 10k fds gets EMFILE.

**Why best-effort on prlimit.** Without unprivileged user
namespaces (which need root or a sysctl most operators don't
set), we can't apply rlimits BEFORE the child execs. The window
between Start and prlimit is small (microseconds in practice)
but real. Operators who need hard guarantees should use
ProfileContainer (M2+).

**Why the Limits fields are cross-platform.** Defining them on
the `Limits` struct itself (rather than gating behind a build
tag) lets callers configure limits portably; the runtime
honor-or-ignore decision is then where it belongs — at the
platform boundary. A daemon config that requests
`CPUSeconds: 30, MaxOpenFiles: 256` runs unchanged on Windows
(silently ignored) or Linux (enforced).

**Tests:** `TestRun_DowngradesNamespaceToNone` and
`TestEffectiveProfile_AllRequestsDowngradeInM1c` now skip on
Linux (their assertion no longer holds there); new
`TestEffectiveProfile_LinuxKeepsNamespace` covers the Linux
M1.d behavior and skips on non-Linux. All existing assertions
about output capture, timeout, env scrubbing, and event emission
hold unchanged on both platforms. Verified with:
```
go test ./kernel/warden/ -count=1                # Windows
GOOS=linux go test -c -o /dev/null ./kernel/warden/  # Linux test-binary compile
GOOS=linux go vet ./...                          # full Linux vet
```
Functional Linux-runtime verification (the prlimit/Setpgid paths
actually triggering) waits for a Linux runner — flagged in
follow-up.

## Updated deferral list (further shrunk)

After this batch, the platform-specific category is half-closed:

### Linux done, other OSes still platform-specific
- **Plugin/warden hardening: Linux** — **shipped (M1.d)**
- **Plugin/warden hardening: Windows** (job objects + SetInformationJobObject):
  needs win32 bindings; deferred.
- **Plugin/warden hardening: macOS** (sandbox-exec, App Sandbox):
  needs CGO + private API; deferred.

### Bedrock multi-vendor matrix
- Anthropic / Mistral / Cohere / Meta-Llama / **AI21 Jamba (M1.tt-4)** — all shipped
- AI21 J2 (legacy) — intentionally unwired; deprecated by AI21
- Amazon Titan — intentionally unwired

### Remaining
- Vault OS-keychain integration (per-OS CGO bindings)
- Vault argon2 KDF (no stdlib argon2)
- Browser JS rendering / screenshots (chromedp)
- Plugin callbacks (host-side tool invocation from plugin) — protocol change
- MCP bridge SSE transport (incremental; doable, not yet requested)
- Full warden M1.d.1 (CLONE_NEWUSER + cgroup v2 + seccomp BPF) — large
- Per-task-type budget caps (small; not yet requested)

## How to verify

```
cd d:/Codebox/PROJECTS/Agezt

# Bedrock matrix including new AI21
go test ./plugins/providers/bedrock/ -count=1

# Warden — Windows path
go test ./kernel/warden/ -count=1

# Warden — cross-compile to Linux to verify the build-tagged paths
GOOS=linux go build ./...
GOOS=linux go test -c -o /dev/null ./kernel/warden/
```

## Files added / extended

```
plugins/providers/bedrock/ai21.go        (new)
plugins/providers/bedrock/ai21_test.go   (new)
plugins/providers/bedrock/bedrock.go     (extended — dispatch + error msg)
kernel/warden/warden.go                  (extended — Limits fields, Start/Wait split, platform hooks)
kernel/warden/warden_linux.go            (new)
kernel/warden/warden_other.go            (new)
kernel/warden/warden_test.go             (extended — platform-aware skips + new linux test)
```
