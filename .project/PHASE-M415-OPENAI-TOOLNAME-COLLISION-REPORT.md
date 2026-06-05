# M415 — OpenAI tool-name collision misrouting (HIGH security/correctness)

## Context
A code-review pass over the OpenAI-compatible API surface and the M279 dotted-tool-
name sanitisation found a HIGH-severity defect: the sanitisation is many-to-one and
its reverse map silently overwrote on collision, so a returned tool_call could be
routed to the WRONG tool.

## The bug
`plugins/providers/openai/openai.go`:
- `sanitizeToolName` maps every non-`[A-Za-z0-9_-]` rune to `_`, so distinct names
  collide: `browser.read` and `browser_read` both → `browser_read` (likewise `a.b`
  vs `a-b` is fine but `a.b` vs `a_b` collide; realistic with namespaced MCP tools).
- `reverseToolNames` built `m[sanitized] = original` with **no collision check** —
  last writer wins, non-deterministically by `tools` slice order.
- `encodeRequest`/`encodeStreamRequest` emitted `sanitizeToolName(t.Name)` per tool,
  so two colliding tools were sent as two function defs with the **same** name (an
  invalid request to strict gateways), and `restoreToolCallNames` mapped a returned
  `tool_call` named `browser_read` back to whichever original overwrote the map.

**Impact:** the agent loop looks up `cfg.Tools[tc.Name]` by the restored name, so a
collision runs the model's (or an attacker's) arguments — crafted for tool A —
against tool B. Silent and non-deterministic. Genuine security/correctness bug.

## The fix
New `wireToolNames(tools) (fwd, rev)` computes an **injective** original→wire mapping:
sanitise, then break any collision with a deterministic numeric suffix (`_2`, `_3`,
…) checked against all wire names already used (including unchanged ones). `rev`
contains only entries where wire differs from the original (an unchanged name routes
to itself). Both encoders now use `fwd[t.Name]` for tool-def names and pass `fwd`
into `canonicalToOA` so a replayed assistant turn's tool calls carry the same
collision-safe names; `reverseToolNames` is reimplemented on top of `wireToolNames`,
so the reverse map matches exactly what was put on the wire. Streaming and
non-streaming share one algorithm → identical wire names.

## Verification
- **`plugins/providers/openai/tool_name_test.go`** `TestWireToolNames_CollisionIsInjective`:
  `browser.read` + `browser_read` get distinct, OpenAI-valid wire names; each
  round-trips back to its OWN original; the encoded request contains exactly one bare
  `"browser_read"` (no duplicate function name). Existing `TestSanitizeToolName`,
  `TestEncodeRequestSanitizesToolNames`, `TestRestoreToolCallNames` still pass
  (non-colliding behaviour unchanged).
  - **Negative control:** removing the suffix-disambiguation loop → both tools map to
    `browser_read` → the test FAILs ("collision not broken"). Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2263** passing (was 2262; +1). CHANGELOG
  Security entry added.

## Other review finding (deferred)
The same review noted a LOW issue: `UsageFor` (cmd/agezt/main.go) does a full
`Journal().Range(...)` linear scan per `/v1/chat/completions` and `/v1/responses`
response, so completion latency grows O(total journal events) and a token holder can
force repeated full-journal walks. Authed surface, no memory-safety issue — real
scaling foot-gun on the hot path. Tracked for a follow-up (bound the scan to the
run's correlation / tail-scan), not bundled here to keep this commit to the HIGH fix.
