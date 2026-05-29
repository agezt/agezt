# Phase Report — Milestone 1.p.y (`check --json` + `check --bench N`)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §3 (Operator UX for catalog-driven providers)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.p.x-REPORT.md](PHASE-M1.p.x-REPORT.md).

## Scope

M1.p shipped the live verification command. M1.p.x added the
multi-provider `--all` table. M1.p.y closes two specific gaps the
previous reports flagged as deferred:

```
agt provider check --json [<id>|--all]
agt provider check --bench N [<id>|--all]
```

- **`--json`** — stable machine-readable output. CI scripts can now
  gate on `summary.failed == 0` without scraping a human-formatted
  table, and dashboard tooling can ingest the same shape.
- **`--bench N`** — run N consecutive probes per provider, report
  min/p50/p95/max latencies. Designed for the
  *which-OpenAI-compatible-vendor-is-fastest-from-here* question
  (Groq vs Cerebras vs SambaNova vs DeepInfra vs Fireworks vs Together):
  the catalog already pins the model id; `--bench` measures the
  vendor.

| Concern | M1.p.y status |
|---|---|
| `--json` for single-provider mode | ✅ |
| `--json` for `--all` mode | ✅ |
| `--bench N` for single-provider mode | ✅ |
| `--bench N` for `--all` mode | ✅ adds MIN/P50/P95/MAX columns |
| `--json --bench N` combine cleanly | ✅ embedded `bench` block in each probe |
| Flag parser rejects typos with a clear error | ✅ |
| `--bench N` requires N ≥ 2 (a "bench" of 1 is just a probe) | ✅ |
| Stats use nearest-rank percentile (matches numpy method='nearest') | ✅ |
| Same `runProbe` path as M1.p (no drift) | ✅ |
| Cost arithmetic unchanged (USD-microcents) | ✅ |
| Sequential probes (per-key rate limits would skew concurrent results) | ✅ documented in `runBench` |
| Help text updated | ✅ |

## Changes

### 1. `parseCheckFlags`

A real flag parser replacing the M1.p.x ad-hoc loop. Recognises
`--all/-a`, `--json/-j`, `--bench N`, `--bench=N`, and one positional
provider id. Returns a `checkFlags` struct or an error.

```go
type checkFlags struct {
    all        bool
    jsonOut    bool
    bench      int    // 0 = single-shot; ≥2 = benchmark
    providerID string // empty = auto-pick
}
```

Failure modes (each tested):

- `--bench` without N
- `--bench 1` (rejected — that's just a probe)
- `--bench abc`
- Unknown `--foo` flag
- Two positional args

A typo now errors at the flag layer instead of being silently
treated as a provider id like `--alll`.

### 2. `runBench` + `computeLatencyStats`

`runBench(entry, lookup, n)` calls `runProbe` N times sequentially,
collecting successful latencies + accumulating cost. Sequential by
design — concurrent probes skew p95 for providers with per-key rate
limits (commonly seen on Groq/Anthropic with `429 Too Many Requests`
on second-or-third concurrent calls).

Progress dots (`.` for success, `x` for failure) print per-probe
when not in `--json` mode, so an operator watching `--bench 10`
against a slow provider sees something happening.

`computeLatencyStats` uses **nearest-rank percentile**: sort
ascending, take `sorted[⌈p·n⌉ − 1]`. Same method as
`numpy.percentile(method="nearest")`. For the small N operators
will actually run (typically 3–10), the choice of percentile method
matters less than just being consistent — but documenting which
method we use lets operators reason about edge cases.

Edge cases pinned by test:
- Empty input → all-zero `latencyStats`
- Single value → min=p50=p95=max=that value
- Pre-shuffled input → same result as sorted (proves the sort runs)

### 3. `--bench` rendering

Single-provider bench:

```
$ agt provider check --bench 5 anthropic
checking anthropic           …
  bench anthropic ×5: .....

bench result for anthropic (model=claude-3-5-haiku-20241022)
  iterations      : 5 (5 ok, 0 failed)
  min / p50 / p95 / max : 49ms / 53ms / 71ms / 88ms
  total cost      : $0.000108 (108000 microcents across 5 successful probes)
```

Multi-provider bench (`--all --bench 3`):

```
STATUS  PROVIDER   FAMILY             MODEL                       MIN    P50    P95    MAX    OK/N  TOTAL_COST
OK      anthropic  anthropic          claude-3-5-haiku-20241022   49ms   53ms   71ms   71ms   3/3   $0.0000648
OK      groq       openai-compatible  llama-3.3-70b-versatile     112ms  118ms  140ms  140ms  3/3   $0.0000135
OK      openai     openai             gpt-4o-mini                 230ms  280ms  340ms  340ms  3/3   $0.0000135
```

The vendor-comparison story is now real: in this synthetic example,
Groq's p50 is half OpenAI's at a smaller cost. An operator can
glance and pick.

### 4. `--json` shape

Stable contract — the goal is that CI scripts can pin to this and
not re-write when we add adapters. Top-level:

```json
{
  "probes":  [ { ...jsonProbe... }, ... ],
  "summary": { "total": N, "ok": N, "failed": N, "skipped": N }
}
```

Per-probe shape:

```json
{
  "provider":         "anthropic",
  "family":           "anthropic",
  "model":            "claude-3-5-haiku-20241022",
  "ok":               true,
  "reply":            "pong",
  "stop_reason":      "end_turn",
  "input_tokens":     12,
  "output_tokens":    3,
  "latency_ms":       53,
  "cost_microcents":  21600,
  "bench": {
    "iterations":            5,
    "successes":             5,
    "failures":              0,
    "min_ms":                49,
    "p50_ms":                53,
    "p95_ms":                71,
    "max_ms":                88,
    "total_cost_microcents": 108000
  }
}
```

Design choices:

- **`latency_ms` and `cost_microcents` are integers, not floats.**
  Same reason `formatMicrocentsUSD` exists: JSON floats lose
  precision on sub-cent values and `jq` users can't reliably
  compare them.
- **`bench` is omitted when not benchmarking.** Avoids `null` /
  `{iterations: 0}` ambiguity for the single-shot path.
- **`reply`/`stop_reason`/`*_tokens` omit on failure.** A failed
  probe doesn't have those fields; not emitting them keeps schema
  validators happy and avoids polluting CI dashboards with empty
  strings.
- **`error` only present on failure.** Same logic in reverse.
- **`provider`/`family`/`model` always present** even on failure,
  so dashboards can route the alert to the right team.

Exit codes preserved from M1.p.x:
- `0` — all probes OK
- `1` — at least one probe failed

CI gate:

```bash
agt provider check --all --json | jq -e '.summary.failed == 0'
```

### 5. Help text additions

```
provider check --bench N [id]         run N probes; report min/p50/p95/max latencies
provider check --json [id|--all]      machine-readable output (CI-friendly)
```

### 6. Refactor: shared table renderer

`renderCheckAllTable` and the new `renderBenchAllTable` both
delegate column alignment to a shared `renderTable(headers, cells, rows)`
helper. Bench just supplies more columns (`MIN/P50/P95/MAX/OK_N/TOTAL_COST`
vs `LATENCY/COST`). No duplicate width logic.

## Tests (added)

- `TestParseCheckFlags` — 17 sub-cases covering all flag forms +
  every documented error mode.
- `TestComputeLatencyStats` — 5 sub-cases including empty, single,
  ten-ascending, unsorted (proves the sort), and three-value.
- `TestProbeToJSON` — happy path with full token/cost fields.
- `TestProbeToJSON_Failure` — failure path; assertss
  response-shaped fields are absent.
- `TestEmitJSON_RoundTrip` — marshal + unmarshal + headline key
  presence (the CI-contract guard).
- `TestBenchToJSON` — bench block matches the latency stats and
  the headline `latency_ms` reflects p50.

All 343 tests pass (up from 315 in M1.p.x).

## Architectural consequences

1. **JSON output is the new public contract.** Before today, the
   only stable surface `agt` exposed to scripts was exit codes. Now
   anything with `--json` is committed-API: rename or remove a
   field, you break a downstream. The shape was chosen
   conservatively (integers for money + duration, omit-on-irrelevant
   for optional fields) precisely because of that lock-in.

2. **Latency stats are tractable because the probe is tiny.**
   `MaxTokens: 16` with a one-word reply means even `--bench 30`
   completes in seconds and costs single-digit cents on the most
   expensive providers. This is the inverse of typical benchmarking
   work — instead of fighting variance with huge sample sizes,
   we keep the call dirt-cheap so operators can run it casually.

3. **Sequential probes are a deliberate choice.** Anthropic and
   Groq both rate-limit aggressively on the same key; concurrent
   probes would surface noise (some get 429'd, others don't), not
   signal. When operators ask "should this be parallel?" the answer
   is: that would measure something different (concurrency
   capacity), not single-call latency.

4. **`--bench --all` re-runs `runProbe` per-provider per-iteration,
   not interleaved.** All N probes for provider A complete before
   probe 1 of provider B starts. This avoids interleaved noise
   (sharing TCP keep-alive state across providers, etc) and matches
   the mental model "give me a clean baseline for provider X".

## Demo transcript (synthetic)

### Single provider, JSON

```
$ agt provider check anthropic --json
{
  "probes": [
    {
      "provider": "anthropic",
      "family": "anthropic",
      "model": "claude-3-5-haiku-20241022",
      "ok": true,
      "reply": "pong",
      "stop_reason": "end_turn",
      "input_tokens": 12,
      "output_tokens": 3,
      "latency_ms": 53,
      "cost_microcents": 21600
    }
  ],
  "summary": { "total": 1, "ok": 1, "failed": 0 }
}
```

### --all --json

```
$ agt provider check --all --json | jq '.summary'
{
  "total":   3,
  "ok":      2,
  "failed":  1,
  "skipped": 46
}
$ echo $?
1
```

### --bench, human

```
$ agt provider check anthropic --bench 5
checking anthropic           …
  bench anthropic ×5: .....

bench result for anthropic (model=claude-3-5-haiku-20241022)
  iterations      : 5 (5 ok, 0 failed)
  min / p50 / p95 / max : 49ms / 53ms / 71ms / 88ms
  total cost      : $0.000108 (108000 microcents across 5 successful probes)
```

### --all --bench --json — full vendor comparison output

```
$ agt provider check --all --bench 5 --json | \
    jq '.probes[] | {provider, p50_ms: .bench.p50_ms, p95_ms: .bench.p95_ms}'
{"provider":"anthropic","p50_ms":53,"p95_ms":71}
{"provider":"groq",     "p50_ms":118,"p95_ms":140}
{"provider":"openai",   "p50_ms":280,"p95_ms":340}
```

That one-liner is the entire vendor-shootout workflow.

## Deferrals → next phase candidates

- **Streaming (SSE)** — the biggest functional gap remaining. Token
  chunks via `Provider.CompleteStream` + new bus event kind +
  CLI rendering. Anthropic-first scope is ~400 LoC, makes long
  completions feel alive.
- **`agt catalog list --json`** — apply the same JSON-contract
  pattern to the catalog command. Small.
- **Hot reload of catalog + vault** — eliminates the "restart the
  daemon" friction printed by `agt provider creds set`.
  Daemon-side; needs Governor `ReplacePrimary` + signal handling.
- **Cache-pricing surfacing** (`cache_read`/`cache_write` in
  `catalog.Cost`) — `--bench` makes this more interesting: cache
  hits should show up as cheaper repeat probes.

Unchanged longstanding deferrals:
- Subscription-first routing (DECISIONS C2).
- OS-keychain encryption for the vault (M1.o.x).
- Bedrock SigV4 + non-Anthropic body shapes (M1.m.x).
- Vertex Anthropic + ADC + workload-identity (M1.n.x).
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
cmd/agt/check.go         (~+220 LoC: parseCheckFlags, runBench, computeLatencyStats,
                          renderBenchAllTable, renderTable, jsonProbe/jsonBench/jsonSummary,
                          probeToJSON, emitJSON; refactor of cmdProviderCheck dispatch)
cmd/agt/check_test.go    (~+220 LoC: 6 new tests, 22 new sub-cases)
cmd/agt/main.go          (+ 2 help lines for --bench and --json)
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 343 pass, 0 fail (up from 315 in M1.p.x)
```

The cumulative operator UX trajectory:

| Milestone | New operator command(s) / capability |
|---|---|
| M1.f | `agt catalog sync`, `agt catalog list`, `agt catalog discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| M1.p.x | `agt provider check --all` |
| **M1.p.y** | **`--json` (CI gate) + `--bench N` (vendor latency comparison)** |
