# Phase Report — Milestone 1.p.x (`agt provider check --all`)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §3 (Operator UX for catalog-driven providers)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.p-REPORT.md](PHASE-M1.p-REPORT.md).

## Scope

M1.p shipped the single-provider verification command. M1.p.x closes
the multi-provider case: one invocation that probes **every catalog
provider with credentials**, prints a summary table, and exits
non-zero if any probe failed.

```
agt provider check --all
```

The motivating workflow: an operator configures Anthropic + OpenAI +
Groq + a local Ollama. They want a single "is my setup still
healthy?" command before they bet a real intent — without typing
`agt provider check <id>` four times.

| Concern | M1.p.x status |
|---|---|
| Iterates catalog providers in deterministic order | ✅ via `cat.ProviderList()` |
| Skips uncredentialed providers (no spurious failures) | ✅ |
| Skips unsupported families (catalog ≠ wire-supported subset) | ✅ |
| Same `runProbe` path as single-provider check (no drift) | ✅ refactored to share |
| Per-row latency + cost matches single-provider output | ✅ same `computeCostMicrocents` + `formatMicrocentsUSD` |
| Non-zero exit when any probe fails (CI-friendly) | ✅ |
| Error detail shown without breaking table alignment | ✅ separate `! <id>: <err>` lines below the table |
| No daemon required | ✅ inherited from M1.p |
| Renderer unit-tested | ✅ 2 new tests |

## Changes

### 1. `cmd/agt/check.go` — refactor + `--all` runner

Extracted the resolve-model → build → Complete → cost path into
`runProbe(entry, lookup) probeResult`. Both `cmdProviderCheck` (the
single-provider path) and the new `cmdProviderCheckAll` call it. The
shared call site is what guarantees the two views report the same
numbers — they can't drift because there's nothing to drift between.

`cmdProviderCheckAll`:

```go
for _, entry := range cat.ProviderList() {
    if !compat.IsSupportedFamily(entry.Family()) { skipped++; continue }
    if !entry.HasCredentials(lookup)              { skipped++; continue }
    res := runProbe(entry, lookup)
    rows = append(rows, checkRow{...})
}
fmt.Fprintln(stdout, renderCheckAllTable(rows))
```

Skip rules:

- **Unsupported family** — catalog includes providers we haven't
  wired yet. Showing them as "FAIL" would be misleading; they're
  legitimately not covered by `compat.Build`.
- **Missing credentials** — same logic the daemon and auto-pick use
  (`Provider.HasCredentials(lookup)`). An operator with only an
  Anthropic key shouldn't see "FAIL: OpenAI no key" — that's not a
  failure, it's just absence.

Both counts surface in the trailer line:

```
3 checked: 2 ok, 1 failed (skipped 47 uncredentialed/unsupported)
```

So operators still see how many providers exist in the catalog
overall without each one occupying a row.

### 2. `--all` flag dispatch

`cmdProviderCheck` accepts `--all` (or `-a`) and short-circuits to
the multi-runner. Putting the flag inside `cmdProviderCheck` rather
than as a separate subcommand keeps the surface area tight — one
command, one mental model, with a flag that changes the iteration
shape.

### 3. Table renderer (`renderCheckAllTable`)

Columns: STATUS / PROVIDER / FAMILY / MODEL / LATENCY / COST.
Auto-widths from the cells themselves, two-space gutters, no
horizontal lines (the eye picks rows out fine and lines waste
columns on narrow terminals).

Error messages don't fit a column and would blow the table width
out of any reasonable terminal. They're emitted as indented lines
below the table:

```
  ! groq: build groq: compat: no credentials available
```

Cost rendering for the three states the operator might see:

| state | cell |
|---|---|
| ok + cost > 0 | `$0.0000216` |
| ok + cost = 0 (Ollama, free models) | `(no price)` |
| failure | `-` |

The third case used to render as `$0.00` in an earlier draft, which
falsely implies "ran for free" — the run didn't happen at all. Test
`TestRenderCheckAllTable` pins the "-" expectation.

### 4. Help text

Added one line to `cmd/agt/main.go` `printHelp`:

```
provider check --all                  probe every credentialed provider; summary table
```

### 5. Tests (`cmd/agt/check_test.go`)

Two new test functions:

- `TestRenderCheckAllTable` — 3-row fixture (anthropic OK with
  cost, openai OK with smaller cost, groq FAIL with error string).
  Asserts header presence, both cost cells, both providers, the
  error trailer line, and that the FAIL row renders cost as `-`
  not `$0`.
- `TestRenderCheckAllTable_EmptyCost` — Ollama-shaped fixture with
  ok=true and cost=0. Asserts `(no price)` appears (not `$0.00`).

## Architectural consequences

1. **`runProbe` is now the single execution surface.** M1.p
   introduced `compat.Build` as the single *resolution* surface;
   M1.p.x adds `runProbe` as the single *execution* surface. The
   single-provider view and the table both call it, so changes to
   probe behavior (prompt, MaxTokens, timeout) flow to both
   automatically. Future work like `--bench N` plugs in here.

2. **CI-friendly exit code.** `--all` returns 1 if any probe
   failed. This is the first Agezt command intended to be wired
   into a CI gate ("nightly: do our credentials still work?"), so
   the contract matters. Skipped providers don't affect exit code —
   only actual failures do.

3. **Skip vs fail is a real distinction.** "Anthropic isn't
   credentialed" is operator state, not a system failure. Conflating
   the two would mean every CI run cried wolf about providers the
   operator hasn't set up yet. The trailer `(skipped N)` keeps the
   number visible without inflating the table.

## Demo (synthetic, no real credits)

Catalog with three providers, only two credentialed:

```
$ agt provider creds list
2 vault entries at <vault path>

  anthropic
    ANTHROPIC_API_KEY                        = sk-f••••••emo
  openai
    OPENAI_API_KEY                           = sk-o••••••est
```

```
$ agt provider check --all
checking anthropic            …
checking openai               …

STATUS  PROVIDER   FAMILY     MODEL                        LATENCY  COST
OK      anthropic  anthropic  claude-3-5-haiku-20241022    53ms     $0.0000216
OK      openai     openai     gpt-4o-mini                  91ms     $0.0000045

2 checked: 2 ok, 0 failed (skipped 47 uncredentialed/unsupported)
$ echo $?
0
```

With one mis-configured provider (e.g. bad endpoint):

```
$ agt provider check --all
checking anthropic            …
checking openai               …
checking groq                 …

STATUS  PROVIDER   FAMILY             MODEL                       LATENCY  COST
OK      anthropic  anthropic          claude-3-5-haiku-20241022   53ms     $0.0000216
OK      openai     openai             gpt-4o-mini                 91ms     $0.0000045
FAIL    groq       openai-compatible  llama-3.3-70b-versatile     312ms    -
  ! groq: openai: 401 Unauthorized

3 checked: 2 ok, 1 failed (skipped 46 uncredentialed/unsupported)
$ echo $?
1
```

## Deferrals → M1.p.x.x and beyond

- **`--bench N`** — N consecutive probes per provider, p50/p95
  latencies. Useful for picking the lowest-latency
  openai-compatible vendor among Groq/Cerebras/SambaNova.
- **`--json`** — machine-readable output for CI dashboards.
- **Concurrent probes** — currently sequential. For 5+ providers
  the wall-clock starts to matter; a bounded-concurrency runner
  would help. Holding off until an operator complains about it.
- **Cache-pricing surfacing** (still deferred from M1.p) —
  `cache_read`/`cache_write` in `catalog.Cost`.

Unrelated deferrals from prior milestones (unchanged):
- Streaming (SSE) uniformly across the 7 wire adapters.
- Subscription-first routing (DECISIONS C2).
- OS-keychain encryption for the vault (M1.o.x).
- Hot reload of catalog + vault (M1.o.x / M1.f.x).
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.
- Bedrock SigV4 + non-Anthropic body shapes (M1.m.x).
- Vertex Anthropic + ADC + workload-identity (M1.n.x).

## Files touched

```
cmd/agt/check.go         (refactor: extract runProbe; +cmdProviderCheckAll; +renderCheckAllTable; ~+170 LoC)
cmd/agt/check_test.go    (+TestRenderCheckAllTable, +TestRenderCheckAllTable_EmptyCost)
cmd/agt/main.go          (+ "provider check --all" help line)
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 316 pass, 0 fail (up from 314 in M1.p)
```

The cumulative operator UX trajectory:

| Milestone | New operator command(s) |
|---|---|
| M1.f | `agt catalog sync`, `agt catalog list`, `agt catalog discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| **M1.p.x** | **`agt provider check --all`** |
