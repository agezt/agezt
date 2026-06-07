# M527 — Mutation testing openaiapi: pin the word-count usage fallback

## Context
Thirty-fourth package in the mutation pass: `kernel/openaiapi` (the OpenAI-compatible HTTP
surface — chat/responses/models, token-auth, oversize guard, vision-input parsing). Large
(1100+ LOC), fuzzed, 7 test files. Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting score
0.524, 185 survivors; tree restored clean.

## Triage — the request surface is well covered
Batch negative control confirmed the security/parse core is solid: `authorized`
(constant-time compare, same pattern verified in restapi M513), `wantsJSON` (both
`json_object` and `json_schema` branches), `images()` / `inputImages()` part filters, and
`imagesFromMessages`'s user-role filter are all killed by the existing tests + fuzz. The
`chatUsage` real-provider path (`pt + ct`) is pinned by `usage_test.go` (1406+11=1417).

## The genuine gap (closed)
`estimateUsage` is the **fallback** usage block, used when the engine is not a
`UsageReporter` (no real provider token counts):

```
p := len(strings.Fields(prompt))
c := len(strings.Fields(completion))
return map[string]any{"prompt_tokens": p, "completion_tokens": c, "total_tokens": p + c}
```

No test calls it directly, and the main usage test uses a `UsageReporter` engine — so it
hits `chatUsage`'s `pt + ct`, never `estimateUsage`. Its `total_tokens: p + c` was
therefore unpinned: `+ → *` and `+ → -` both survived (a two-word completion would report
a product or difference as the total — wrong usage/billing numbers for any client relying
on the heuristic when the provider gives no counts).

## Fix
Added `TestEstimateUsage_WordCount` (direct unit test): `estimateUsage("one two three",
"four five")` → prompt 3, completion 2, total 5 — pinning the sum, not a product/difference.

## Negative control (manual, CPU-capped)
`total_tokens: p + c → p * c` and `→ p - c` each FAIL under the new test. Restored
byte-for-byte (`git diff --ignore-all-space` on openaiapi.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-four packages (M490–M527)
…pulse, openaiapi — plus the controlplane primary-token auth gate verified solid. The
openaiapi request/parse/auth surface was already well covered (fuzz + 7 test files); the
gap was the value-untested word-count usage fallback.
