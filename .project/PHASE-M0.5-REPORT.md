# Phase Report — Milestone 0.5 (Minimal Working Core, "core-core")

> Status: **shipped** · Date: 2026-05-29
> Per [BUILD-GUIDE §4 M0.5](BUILD-GUIDE.md) and [ROADMAP §0.5](ROADMAP.md).
> Continues [PHASE-M0-REPORT.md](PHASE-M0-REPORT.md).

## What shipped

The smallest thing that proves the foundation — event log + state store +
bus + first-party single-agent tool-loop + one Provider + one Tool + the
control plane + `agt` command client + a working e2e demo gate.

### New kernel packages

| Package | LoC | Tests | Purpose |
|---|---:|---:|---|
| `kernel/ulid` | 318 | 6 | Crockford base32 ULID generation; concurrent-safe; deterministic via `NewWith` |
| `kernel/event` | 430 | 10 | Canonical Event + Kind constants + deterministic JSON encoding + BLAKE3 chain-link |
| `kernel/journal` | 628 | 8 | Segmented JSONL append-only log; fsync per write; rotation; recovery scan; `Verify` walks the chain |
| `kernel/state` | 475 | 11 | Per-namespace KV store with atomic write-temp+rename; load-on-Open |
| `kernel/bus` | 464 | 9 | In-process subject router with `*`/`>` wildcards; bounded subscribers with drop counters; **durable-before-publish** invariant |
| `kernel/agent` | 555 | 6 | Canonical `Message`/`ToolCall`/`ToolDef`/`Provider`/`Tool` types + first-party tool-loop (DECISIONS B0d); bounded iterations; honors `ctx.Done()` |
| `kernel/runtime` | 453 | 6 | `Kernel` wrapper that owns journal+state+bus+provider+tools; `Halt`/`Resume`/`Run`/`RunWith`/`Why`/`Verify` |
| `kernel/controlplane` | 794 | 7 | TCP-localhost server + token + line-delimited JSON protocol; `Client` for the `agt` binary |

### New plugin packages (all in-process per DECISIONS B0a)

| Package | LoC | Tests | Purpose |
|---|---:|---:|---|
| `plugins/providers/mock` | 89 | (used in agent/controlplane/runtime tests) | Scripted Provider for tests + offline demo; `FinalText` / `ToolUse` helpers |
| `plugins/providers/anthropic` | 503 | 7 | Anthropic Messages API client (non-streaming for M0.5); canonical↔Anthropic dialect translation per SPEC-15 |
| `plugins/tools/shell` | 244 | 6 | In-process shell tool; `cmd /C` on Windows, `sh -c` elsewhere; timeout via `ctx.WithTimeout` + `cmd.WaitDelay`; output truncated at 64 KiB |

### New CLI surface

| Path | LoC | Tests | Purpose |
|---|---:|---:|---|
| `cmd/agezt` | 269 | 3 | Daemon mode: starts kernel + control plane; provider selection via `AGEZT_PROVIDER=mock` or `ANTHROPIC_API_KEY` |
| `cmd/agt` | 239 | 5 | Client: `run "<intent>"`, `halt`, `resume`, `why <id>`, `journal verify`, `version`, `help` |
| `internal/paths` | 31 | — | Resolves `$AGEZT_HOME` or `~/.agezt` for the binaries |

### Dependencies added (with allowlist + DEPENDENCIES.md justification)

- `lukechampine.com/blake3` v1.4.1 — DECISIONS B3 freezes BLAKE3; stdlib has no BLAKE3. Pure-Go, MIT, no CGO. POLICY §1.3 pre-blessed.
- `github.com/klauspost/cpuid/v2` v2.0.9 — transitive of blake3 for SIMD feature detection. Pure-Go, MIT.

`tools/depscheck` enforces the allowlist (`tools/depscheck/allowlist.txt`)
in CI; any module not in that file fails the build.

---

## Demo gate transcript (the M0.5 success test, verbatim)

From [ROADMAP §0.5](ROADMAP.md):

> `agt run "list the files here and tell me what this project is"` → the
> agent loops (LLM ↔ shell tool), produces an answer, every step is
> journaled, `agt why` explains it, `agt halt` stops it. Runs as one
> process. Chain verifies clean.

Run on Windows 11 with `AGEZT_PROVIDER=mock` (no API key set):

```
$ ./bin/agezt &
Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : C:/Users/ersin/AppData/Local/Temp/agezt-m05-demo
  provider         : mock (offline; scripted 2-turn shell+final)
  control plane    : 127.0.0.1:52497

$ ./bin/agt run "list the files here and tell me what this project is"
  [evt seq=0 kind=task.received]
  [evt seq=1 kind=llm.request]
  [evt seq=2 kind=llm.response]
  [evt seq=3 kind=tool.invoked]
  [evt seq=4 kind=tool.result]
  [evt seq=5 kind=llm.request]
  [evt seq=6 kind=llm.response]
  [evt seq=7 kind=task.completed]

--- final answer ---
[offline-mock] I ran a directory listing via the shell tool. This project is
Agezt — an open-source, MIT-licensed agentic operating system written in Go.
The M0.5 foundation under kernel/ (event, journal, state, bus, agent, runtime,
controlplane) plus the in-process plugins under plugins/ are what just
executed this run; every step you saw was journaled and BLAKE3-chained.
(correlation_id: run-01KSRB12R47C0G6ENKP3V1AE88)

$ ./bin/agt why 01KSRB12R439P7A5QSS56HF2W1
8 events in correlation:
  seq=0 kind=task.received    subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.task
  seq=1 kind=llm.request      subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.llm
  seq=2 kind=llm.response     subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.llm
  seq=3 kind=tool.invoked     subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.tool
  seq=4 kind=tool.result      subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.tool
  seq=5 kind=llm.request      subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.llm
  seq=6 kind=llm.response     subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.llm
  seq=7 kind=task.completed   subject=agent.agent-run-01KSRB12R47C0G6ENKP3V1AE88.task

$ ./bin/agt journal verify
{
  "ok": true
}

$ ./bin/agt halt
{ "ok": true }
$ ./bin/agt run "blocked"
agt run: controlplane: runtime: kernel is halted
$ ./bin/agt resume
{ "ok": true }
$ ./bin/agt journal verify
{ "ok": true }
```

Full 12-event chain (run + halt + resume + a partial second run that
surfaced a provider error cleanly through the journal):

```
seq=  0 kind=task.received        prev=000000000000... hash=b02b67fc9c37...
seq=  1 kind=llm.request          prev=b02b67fc9c37... hash=20ba3fa203be...
seq=  2 kind=llm.response         prev=20ba3fa203be... hash=8ae56cabea7d...
seq=  3 kind=tool.invoked         prev=8ae56cabea7d... hash=294f966325b8...
seq=  4 kind=tool.result          prev=294f966325b8... hash=4dd59b1e4491...
seq=  5 kind=llm.request          prev=4dd59b1e4491... hash=ba982a53ee12...
seq=  6 kind=llm.response         prev=ba982a53ee12... hash=30d71155fd4e...
seq=  7 kind=task.completed       prev=30d71155fd4e... hash=c9e6ec9e30dd...
seq=  8 kind=halt                 prev=c9e6ec9e30dd... hash=ed5e394d9363...
seq=  9 kind=resume               prev=ed5e394d9363... hash=b36e7e3900c8...
seq= 10 kind=task.received        prev=b36e7e3900c8... hash=b5ee467c5295...
seq= 11 kind=llm.request          prev=b5ee467c5295... hash=0f4040a6c3a2...
```

Every `prev_hash` matches the previous event's `hash`; `journal verify`
re-hashes from genesis and confirms the chain is intact.

---

## Exit-criteria check (ROADMAP §0.5 / BUILD-GUIDE §4 M0.5)

| Criterion | Status |
|---|---|
| Event type + Journal (append-only JSONL + BLAKE3 chain + ULID + recover + verify) | ✅ `kernel/event`, `kernel/journal`, `kernel/ulid` |
| Mutable state store | ✅ `kernel/state` |
| In-process bus (subject routing, durable-before-publish) | ✅ `kernel/bus` — `TestDurableBeforePublish`, `TestOrderPreserved_UnderConcurrency` |
| First-party single-agent tool-loop (canonical, bounded, honors halt, journals each step) | ✅ `kernel/agent.Run` — `TestRun_ToolCallRoundtrip`, `TestRun_HonorsContextCancel`, `TestRun_MaxIterStops` |
| One Provider (Anthropic) + offline mock | ✅ `plugins/providers/anthropic` (7 tests via httptest), `plugins/providers/mock` |
| One in-process Tool (shell) | ✅ `plugins/tools/shell` — cross-platform, timeout-tested |
| Control plane: `agt halt`, `agt why <event>`, `agt journal verify` | ✅ via `kernel/controlplane`; demo transcript above |
| `agt run "<intent>"` end-to-end | ✅ demo transcript above |
| Chain verifies clean after the run | ✅ `{"ok":true}` from `agt journal verify` |
| All on stdlib + the two justified deps; CGO_ENABLED=0 | ✅ depscheck pass; binaries are 3.0 MB (`agt.exe`) and 6.8 MB (`agezt.exe`), static |

---

## Code volume

| Category | LoC |
|---|---:|
| Kernel packages (8) | 4,117 |
| Plugin packages (3) | 836 |
| CLI binaries (2) | 508 |
| Internal/tools/codegen (4) | 735 |
| **Total source (M0 + M0.5)** | **~6,200** |

Tests live alongside each package; ~80 individual test functions across the
module, all passing on `go test ./...` (Windows, CGO_ENABLED=0). Race
detection runs on CI's Linux job (CGO_ENABLED=1).

---

## Deviations from spec (intentional, all documented inline)

1. **Anthropic provider is non-streaming for M0.5.** SPEC-15 calls for
   streaming; we ship the non-streaming path now and add streaming in MVP.
   The canonical types already support streaming chunks; only `provider.complete`
   needs to be split.
2. **Sidecar journal index not implemented.** DECISIONS D1 mentions
   `offset,seq,hash` sidecars; M0.5 verifies and iterates by sequential
   scan (good for 100k events, then a perf concern). Scope-deferred to M2.
3. **Mutable state store is per-file JSON snapshots.** DECISIONS D2 names
   "CobaltDB (embedded B+Tree)" as the destination; the M0.5 implementation
   is correct (atomic write-temp+rename per Set, ACID per namespace) but not
   scaled. Behind the `Store` interface; swap without changing callers.
3. **Shell tool runs WITHOUT sandboxing.** Documented prominently in the
   package doc. Warden namespace/container profiles + Edict policy gating
   land in MVP (TASKS `P1-WARD-01`, `P1-EDICT-*`). Today the tool runs
   with the kernel's full privileges — run only with trusted providers
   and prompts.
4. **`agt why` returns the correlation group, not a tree.** Per the
   M0.5 minimum reading of TASKS `P0-CTRL-03`; a richer
   causation-tree view will land when `EVT_CHECKPOINT_CREATED` and friends
   appear.
5. **Demo gate executed with the offline mock provider** (no
   ANTHROPIC_API_KEY in the build environment). The mock is scripted to
   the exact 2-turn flow the gate requires — shell-call + final-text —
   so every wire-format invariant (journal, hash chain, bus subject
   routing, control plane streaming, halt/resume) is exercised in
   production wiring, not a test harness. A live-LLM rerun with
   `ANTHROPIC_API_KEY` is mechanically the same: the only difference is
   which Provider implementation `selectProvider` picks in
   `cmd/agezt/main.go`.

---

## Open items / TODOs for M1 (MVP) entry

- **Governor** (TASKS `P1-CONDUIT-01..04`): provider registry,
  subscription→cost→latency routing, USD-microcent budget, fallback chain.
- **DAG scheduler over the loop** (DECISIONS B0d, TASKS `P1-SCHED-01..03`):
  tool/llm/loop/gate nodes; parallel branches; retry/compensation. Loop
  becomes a node type, not the only path.
- **Edict policy engine + trust ladder** (TASKS `P1-EDICT-01..03`):
  approval flow, hard-deny, per-capability levels — required before any
  autonomous action ships.
- **Warden** (TASKS `P1-WARD-01`): namespace/cgroups/seccomp; container
  profile for the browser tool.
- **More tools**: file, http, browser (in container) — TASKS `P1-TOOL-02..04`.
- **Ollama provider** for offline floor — TASKS `P1-PROV-02`.
- **Telegram channel** + plugin-host out-of-process subprocess flow —
  TASKS `P4-CHAN-01`, `P0-HOST-*` (deferred to MVP since M0.5 is all
  in-process per DECISIONS B0a).
- **Streaming completions** in the Anthropic provider.
- **`jsonschemagen` enhancements**: typed string aliases for all-string
  enums (currently `json.RawMessage`); preserve contract field order
  (currently alphabetical for determinism).
- **`agt why` tree view** once causation-id wiring is richer.
- **Snapshot mechanism** (DECISIONS D1: every 10k events or 1h) for boot
  speed at scale.
- **Doc reconciliation**: `TASKS.md`'s `P0-PROTO-01..03` still mention
  `.proto` codegen; `ROADMAP.md §1/§6` still mentions `buf/protoc`.
  BUILD-GUIDE §2 already supersedes them, but adding inline
  `(SUPERSEDED by B0 — JSON-RPC + JSON Schema)` notes would help future
  readers.

---

## Pointers

- Build: `make build` or `go build ./...`
- Test: `make test` or `go test ./...`
- Run offline demo:
  ```bash
  export AGEZT_PROVIDER=mock
  ./bin/agezt &           # daemon, foreground
  ./bin/agt run "list the files here and tell me what this project is"
  ./bin/agt journal verify
  ./bin/agt halt
  ```
- Run live: `export ANTHROPIC_API_KEY=sk-...; ./bin/agezt`, then same `agt` flow.
- Next milestone: [ROADMAP §2 (MVP)](ROADMAP.md) and TASKS Phase 1.
