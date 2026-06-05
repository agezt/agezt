# M443 — UsageFor fast path: O(journal) → O(1) per API usage response

## Context
Resolving the last substantive deferred item: *"UsageFor scans the full journal
per API usage response."* The earlier deferral was because a naive fix risked
billing-correctness; this implementation is correctness-safe **by construction**.

## The problem
`kernelAPIEngine.UsageFor(corr)` (the `usage` field of every OpenAI-compat
chat/responses reply) called `Journal().Range(...)` — a full journal scan —
summing `budget.consumed` events for the correlation. That is **O(journal) per
API response**: on a long-lived daemon the journal grows without bound, and an
authenticated client hammering the API amplifies each request into a full-journal
scan (a CPU/IO DoS vector).

## The fix (correctness-safe by construction)
The Governor already computes each call's `inTok/outTok` immediately before it
emits `budget.consumed`. It now also records them into a bounded in-memory
per-correlation index (`indexUsageTokens`), summed exactly as the journal fold
does. `kernelAPIEngine.UsageFor` consults that index first (via a type assertion
on `k.Provider()`, which is the Governor) and **falls back to the existing journal
scan on any miss**.

Why it cannot regress the reported numbers:
- The index sums the *same* `inTok/outTok` that go into `budget.consumed`, so a
  hit equals the journal sum exactly.
- A miss (unknown or evicted correlation) runs the unchanged authoritative scan.
- The same "found-but-zero → false" rule is applied on both paths.
- The index is **reporting-only** — it is never read for ceiling enforcement or
  billing (that remains `spentToday`), and lives behind its own `usageMu`, never
  touching the spend hot path.

Bounding: when the index exceeds `usageIndexCap` (8192) it is dropped wholesale —
an evicted correlation simply misses and falls back to the journal, never a wrong
sum, and there is no slice-growth/eviction-bookkeeping pathology. 8192 dwarfs any
realistic in-flight set, so "usage for the run that just finished" (the only
caller pattern) is effectively always a hit. Per-tenant kernels read their own
tenant Governor's index (`tk.Provider()`), so attribution stays correct.

## Verification
- **`kernel/governor/usage_index_internal_test.go`** (white-box):
  - `TestUsageIndex_AccumulatesAndReports`: tokens sum across a run's calls
    (10+3, 5+2 → 13/7); distinct correlations independent; empty correlation
    ignored; unknown correlation misses (→ caller falls back to the journal).
  - `TestUsageIndex_Bounded`: after `usageIndexCap+50` correlations the index
    stays ≤ cap and non-empty (retains recent entries).
  - **Negative control:** neuter the `e.in += in; e.out += out` accumulation →
    `UsageFor` returns `(0,0)` and AccumulatesAndReports FAILs. Restored.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2317** passing (was 2315;
  +2), `go test ./...` exit 0 (the existing openaiapi/restapi UsageFor tests,
  which exercise the journal-fallback path through their mock engines, still
  pass). CHANGELOG Reliability entry.

## Review status
This closes the last substantive offline-actionable item. Remaining deferrals are
deliberate design choices (slack/discord `context.Background` detach, anthropic
strict stream abort) or external-wire/by-design (embeddings, cloud creds,
Docker/CI, env-only config, journal-0644, plaintext non-loopback).
