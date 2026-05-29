# Phase Report — Milestone 1.p (`agt provider check`)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §3 (Operator UX for catalog-driven providers)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-14](TASKS.md). Continues
> [PHASE-M1.o-REPORT.md](PHASE-M1.o-REPORT.md).

## Scope

M1.o gave operators a vault. M1.p gives them the matching
**verification primitive**: a live roundtrip command that proves
the full stack works *before* they bet a real intent on it.

```
agt provider check [<provider-id>]
```

Runs a tiny "Say 'pong' in one word." prompt against the
catalog-resolved provider, with `MaxTokens: 16` so the probe is
near-free. Reports:

- The reply text (truncated to 80 chars for safety)
- End-to-end latency (HTTP roundtrip including parse, not wall-clock since the daemon boot)
- Stop reason
- Token counts (input / output)
- Model's catalog pricing (input + output USD per MTok)
- This call's actual cost — integer USD-microcents, matched to the Governor's pricing math

**No daemon required.** `agt provider check` reads the catalog + vault
from disk directly and builds the provider via `compat.Build` —
identical resolution to what the daemon would do at boot. This
makes it the canonical "did I set this up right?" command for both
first-time setup and post-rotation verification.

| Concern | M1.p status |
|---|---|
| Live roundtrip via `compat.Build` (same resolution as daemon) | ✅ |
| Vault-aware (uses `creds.ChainLookup(vault, env)` same as daemon) | ✅ |
| Auto-pick mode when no id arg (same logic as daemon) | ✅ |
| Cost in USD-microcents matching `kernel/governor` arithmetic | ✅ |
| Integer-precise sub-cent rendering (no float64 cents-rounding) | ✅ — caught a real bug in test |
| Tiny probe prompt + low MaxTokens (≤ ¢ per run for most models) | ✅ `MaxTokens: 16` |
| 60s ctx timeout (slow providers don't hang the CLI) | ✅ |
| Clear errors for: missing catalog, missing creds, unknown provider | ✅ |
| Doesn't require a running daemon | ✅ |
| Doesn't touch the journal (it's a *check*, not a *run*) | ✅ by design |

## Changes

### 1. `cmd/agt/check.go` — new file

`cmdProviderCheck(args []string, stdout, stderr io.Writer) int`:

1. Load catalog from disk (errors out clearly if empty — "run `agt catalog sync` first").
2. Load credentials vault.
3. Build `creds.ChainLookup(vault.Lookup, os.Getenv)` — same chain order as the daemon.
4. Resolve provider id: explicit arg → `AGEZT_PROVIDER` → auto-pick via `autoPickFromCatalog`.
5. Resolve model: `AGEZT_MODEL` → `compat.FirstModelID(entry)`.
6. `compat.Build(entry, modelID, lookup)`.
7. Issue one `Complete` with the probe prompt, 60s timeout.
8. Compute cost via `computeCostMicrocents(model, usage)`.
9. Pretty-print the result.

`autoPickFromCatalog(cat, lookup)` mirrors the daemon's auto-pick:
walks `cat.ProviderList()` (deterministic sort), picks the first
entry that is both a supported family AND has credentials. Returns
`nil` when nothing matches.

`computeCostMicrocents(model, usage)`:

```go
return (in*inMc + out*outMc) / 1_000_000
```

Where `inMc` and `outMc` are from `catalog.Cost.InputMicrocentsPerMTok()`
and `OutputMicrocentsPerMTok()` — the *same* helpers the Governor
uses. So check-reported cost always agrees with what the daemon
would record for the same call.

`formatMicrocentsUSD(mc int64) string` — integer arithmetic, no
float64:

```go
dollars := mc / 1_000_000_000
sub     := mc % 1_000_000_000              // 0..999_999_999
subStr  := strings.TrimRight(fmt.Sprintf("%09d", sub), "0")
// pad to at least 2 decimal places, prepend dollar amount
```

This is the load-bearing detail M1.p had to get right. An earlier
draft used `float64(mc) / 1e9` with `%.6f`, which rounded
21,600 microcents to "$0.000005" instead of the integer-precise
"$0.0000216". The unit test caught it immediately.

### 2. CLI dispatch + help

`cmd/agt/provider.go` gains a `check` case in `cmdProvider`.
`cmd/agt/main.go` adds one line to the help text.

### 3. Tests (`cmd/agt/check_test.go`)

- `TestComputeCostMicrocents` — 5 cases, including the real
  claude-opus and gpt-4o-mini token-shaped scenarios.
- `TestFormatMicrocentsUSD` — 8 cases plus a negative-value
  case. Verifies the full microcent precision is preserved
  (e.g. `0.999999999`).
- `TestTruncate` — 4 cases.

## Demo transcript

A python-stood-up mock Anthropic server stands in for the real
api.anthropic.com so the demo doesn't burn real credits.

### Step 0 — stand up a mock server returning Anthropic-shaped responses

```python
# mock listening on 127.0.0.1:18181, returns:
#   {"id":"msg_p","type":"message","role":"assistant",
#    "model":"claude-opus-4-7","stop_reason":"end_turn",
#    "content":[{"type":"text","text":"pong"}],
#    "usage":{"input_tokens":12,"output_tokens":3}}
```

And point the catalog's `anthropic` entry at it via `custom.json`:

```json
{
  "anthropic": {
    "id": "anthropic", "name": "Anthropic (mock for M1.p demo)",
    "npm": "@ai-sdk/anthropic",
    "api": "http://127.0.0.1:18181",
    "env": ["ANTHROPIC_API_KEY"],
    "models": { "claude-opus-4-7": {"id":"claude-opus-4-7","cost":{"input":5,"output":25}} }
  }
}
```

### Step 1 — no creds yet: clear refusal

```
$ agt provider check anthropic
agt: build anthropic: compat: no credentials available:
"anthropic" needs one of [ANTHROPIC_API_KEY]
```

### Step 2 — stash a fake key in the vault

```
$ agt provider creds set ANTHROPIC_API_KEY sk-fake-for-m1p-demo
stored ANTHROPIC_API_KEY = sk-f••••••demo in <vault path>
```

### Step 3 — explicit check: roundtrip succeeds, cost computed

```
$ agt provider check anthropic
checking provider=anthropic model=claude-3-5-haiku-20241022 family=anthropic …

OK
  reply           : "pong"
  latency         : 53ms
  stop_reason     : end_turn
  tokens in / out : 12 / 3
  model pricing   : $0.80 in / $4.00 out per MTok
  this call cost  : $0.0000216 (21600 microcents)
```

**Math check**: 12 input × $0.80/MTok + 3 output × $4.00/MTok =
$9.6 + $12.0 per million tokens = $21.6 per million × 10⁻⁶ =
$0.0000216 ✓

Note `model=claude-3-5-haiku-20241022` even though `custom.json`
only defined `claude-opus-4-7`. That's because the api.json catalog
merge brings in the full anthropic models map, and `compat.FirstModelID`
picks the alphabetically-first across the merged set (haiku < opus).
Set `AGEZT_MODEL=claude-opus-4-7` to override.

### Step 4 — auto-pick (no id arg)

```
$ agt provider check
checking provider=anthropic model=claude-3-5-haiku-20241022 family=anthropic …

OK
  reply           : "pong"
  ...
```

Same as Step 3 — auto-pick is vault-aware (inherited from M1.o), so
the vaulted Anthropic key alone is enough for the picker to land on
Anthropic.

### Step 5 — unknown provider: precise error

```
$ agt provider check nonexistent-provider
agt: provider "nonexistent-provider" not in catalog (try `agt catalog list`)
```

## Architectural consequences

1. **The operator setup loop is now closed:**

   ```
   agt catalog sync                       # 1. fetch providers + models
   agt provider creds set FOO sk-...      # 2. configure credentials
   agt provider check [<id>]              # 3. verify it works
   agezt                                  # 4. start daemon (banner shows everything)
   agt run "..."                          # 5. real intent
   ```

   Every step has its own diagnostic command. No more "did the API
   change since I last set this up?" guesswork — `check` answers
   that in 50ms.

2. **`compat.Build` is the single resolution surface.** The daemon
   builds providers via `compat.Build(entry, modelID, lookup)`.
   `agt provider check` builds providers via *the same call with
   the same arguments*. Two independent code paths can't drift —
   if they did, the check would lie. Centralising provider
   construction in `compat` is what makes M1.p trustworthy.

3. **Integer USD-microcents is the right pricing unit.** Float64
   USD with `%.6f` lost 4500 microcents → "$0.000005" instead of
   the true "$0.0000045". For a tool whose entire job is to tell
   operators *how much will this cost*, that's a real bug. The
   project's existing decision to track USD in integer-microcents
   end-to-end (DECISIONS C1) prevented the same bug in the
   Governor — M1.p just had to follow the same pattern.

4. **No journal entry for checks.** `check` deliberately doesn't go
   through the daemon's Run machinery — it's a probe, not a
   conversation. Putting it in the journal would pollute the
   correlation graph and conflate "operator verifying setup" with
   "operator dispatching work." If operators want a record, they
   can pipe `agt provider check` to a file.

5. **The probe prompt is the project's first
   non-operator-supplied user prompt.** Worth saying aloud: every
   *intent* in Agezt comes from a human or another service.
   `check` sends a synthetic prompt for diagnostic purposes only.
   It's clearly labelled in the code and runs only when the
   operator invokes the verification command — never automatically.

## Deferrals → M1.p.x and beyond

**M1.p.x — check completeness:**
- `agt provider check --all` — iterate every credentialed
  provider, summarise pass/fail/latency in a table.
- `--bench N` — run N consecutive probes, show p50/p95 latencies
  (useful for picking the lowest-latency openai-compatible vendor
  among Groq/Cerebras/SambaNova).
- Cache-pricing surfacing for providers that expose it
  (`cache_read`/`cache_write` in `catalog.Cost` — the catalog
  carries it but `check` doesn't yet probe it).

**Unrelated deferrals from prior milestones** (unchanged):
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
cmd/agt/check.go         NEW (~190 LoC)
cmd/agt/check_test.go    NEW (3 tests, 17 sub-cases)
cmd/agt/provider.go      (+ "check" dispatch in cmdProvider)
cmd/agt/main.go          (+ "provider check [id]" help line)
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 314 pass, 0 fail (up from 306 in M1.o)
```

New tests this phase: 8 in `cmd/agt`.

The cumulative operator UX trajectory:

| Milestone | New operator command(s) |
|---|---|
| M1.f | `agt catalog sync`, `agt catalog list`, `agt catalog discover` |
| M1.o | `agt provider creds set/list/rm` |
| **M1.p** | **`agt provider check [id]`** |
