# M279 — Fix: OpenAI-compatible providers rejected every tool-bearing request

## Why (the bug, found against a REAL gateway)
The user pointed Agezt at a live OpenAI-compatible gateway (gpt-5.5). Every
`agt run` *appeared* to work but the answer came from the **mock**, not the real
model: `budget.consumed` showed `provider: mock` and the journal carried a
`provider.fallback` event with the real reason:

```
openai: status 400: Invalid 'tools[6].name': string does not match pattern
'^[a-zA-Z0-9_-]+$'.
```

Agezt's `browser.read` tool has a **dot** in its name. OpenAI (and strict
openai-compatible gateways) require tool names to match `^[a-zA-Z0-9_-]+$` and
reject the *entire* request with a 400. The always-on mock fallback caught that
error and silently served the run from the mock — so **no run against a real
OpenAI-compatible provider ever used a tool**, and it was invisible without
inspecting `provider.fallback`. This would break real OpenAI too; it survived
because prior end-to-end testing leaned on the mock.

## What
- **`plugins/providers/openai/openai.go`** — new helpers:
  - `sanitizeToolName(name)` — replaces every rune outside `[a-zA-Z0-9_-]` with
    `_` (so `browser.read` → `browser_read`).
  - `reverseToolNames(tools)` — builds the sanitized→original map (only for names
    that change).
  - `restoreToolCallNames(resp, rev)` — rewrites a response's tool-call names back
    to the originals, in place.
  - Applied: tool **defs** in `encodeRequest` and assistant tool-call **history**
    in `canonicalToOA` now emit sanitised names; `Complete` maps response
    tool-call names back via `reverseToolNames(req.Tools)`.
- **`plugins/providers/openai/streaming.go`** — same on the streaming path: tool
  defs in `encodeStreamRequest` sanitised; `CompleteStream` maps the assembled
  tool-call names back. (Assistant history is shared via `canonicalToOA`.)

The fix is transparent: the kernel keeps using `browser.read`; only the wire form
the model sees is `browser_read`, and tool calls route back correctly.

## Files
- `plugins/providers/openai/openai.go`, `streaming.go` — sanitise/restore (edited).
- `plugins/providers/openai/tool_name_test.go` — 3 tests (new): `sanitizeToolName`
  cases; `encodeRequest` emits the sanitised name and never the dotted one; the
  reverse map + `restoreToolCallNames` round-trips `browser_read` → `browser.read`
  while leaving `shell` untouched.

## Verification
- `go test ./plugins/providers/openai/` — green; full suite **1884 → 1887**
  (+3), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet` clean; `GOOS=linux build` clean;
  `go.mod` / `go.sum` unchanged.
- **Live-proven against the real gateway** (gpt-5.5, before vs after):
  - *before*: `agt run "17 × 23"` → answer from mock, `provider.fallback` logged.
  - *after*: `agt run "What is 17 times 23?"` → **391**, `budget.consumed
    provider=localgw model=gpt-5.5 in=1415 out=23 cost=$0.0015`, **no
    provider.fallback**; and a multi-turn **tool-using** run
    (`shell` → date → answer) completed on the real model with real spend
    ($0.0047 across 2 runs), arc `tool.invoked(shell) → tool.result → task.completed`.

## Scope notes
- Diagnosed by isolating the layers: the provider's `Complete`, `CompleteStream`,
  and `CompleteStream`+tools all worked directly; the failure only appeared in
  the agent loop, which sends the full tool set — narrowing it to the tool-name
  encoding.
- The mock fallback masking a real provider error is itself a sharp edge; this
  fix removes the trigger. (A future hardening could surface `provider.fallback`
  in `agt status`/`doctor` so silent fallbacks are visible — noted for later.)
- No new dependency; behaviour for tools whose names already match the pattern is
  byte-identical (the reverse map is empty, restore is a no-op).
