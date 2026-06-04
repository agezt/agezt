# M327 — Bedrock token usage from response headers

## Why
Two Bedrock vendor adapters — **Mistral** and **Cohere** — return no token counts
in their response bodies (only the answer). Their decoders set `Usage` to just the
model id, so the governor saw **zero spend** for those runs: cost accounting was
wrong and per-run budget caps couldn't fire. The Mistral adapter's own comment
flagged this as a known gap ("counts as zero spend, which is wrong … a future
M1.tt.x can wire those"). Bedrock actually reports the authoritative counts in
response *headers* for every InvokeModel call; this wires them in.

## What
- **`plugins/providers/bedrock/bedrock.go`**: after `Complete` decodes the body,
  if the decoded `Usage` has zero input AND output tokens, it overlays the counts
  from Bedrock's `X-Amzn-Bedrock-Input-Token-Count` /
  `X-Amzn-Bedrock-Output-Token-Count` response headers (via a small
  `headerTokenCount` helper that fails safe to 0 on a missing/garbled header).
  - Vendors with **no** inline usage (Mistral, Cohere) now report real spend.
  - Vendors with inline usage (Anthropic, Nova, Meta-Llama, AI21 Jamba) are
    untouched — the "fill only when zero" rule preserves their richer body-derived
    counts (e.g. Anthropic's cache-read/write breakdown), which the two headers
    can't express.
- **`plugins/providers/bedrock/mistral.go`**: the stale "usage is zero … harmless
  until we plumb the headers through" comment is updated to describe the overlay.

## Verification
- **`plugins/providers/bedrock/mistral_test.go`** (2 tests):
  - Mistral run with `X-Amzn-Bedrock-*-Token-Count` headers → `Usage` carries the
    header counts (37/14), model id preserved.
  - Nova run whose body has inline usage (5/3) and *disagreeing* headers (999/999)
    → inline counts win (the overlay must not override inline usage).
- All pre-existing Bedrock vendor tests unchanged and green (the overlay is a
  no-op when usage is already non-zero, and the prior Mistral/Cohere tests didn't
  send the headers, so they still see zero — behaviour unchanged for them).
- **Live (offline httptest)**: real `bedrock.Provider` vs a mock Mistral endpoint
  returning the token-count headers → `Usage` reports input=128 / output=42
  (previously 0/0). Network-free.
- Full suite **2018** passing, `go test ./...` exit 0 (two consecutive clean runs;
  one earlier run showed an unrelated transient flake in another package — a
  usage-header overlay scoped to the Bedrock provider cannot affect other
  packages, and the bedrock package passes consistently); `gofmt -l` clean;
  `go vet` clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- The overlay is centralized in `Complete`, so it covers every present and future
  Bedrock vendor uniformly without touching per-vendor decoders.
- Streaming (`InvokeModelWithResponseStream`) carries the token counts in the
  terminal `amazon-bedrock-invocationMetrics` event rather than headers; the
  non-Anthropic vendors are non-streaming on this path today, so that's a separate
  follow-up if/when their streaming lands.
