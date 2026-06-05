# M481 — Gemini tool-result JSON built with strconv.Quote → invalid JSON (HIGH)

## Context
The Google/Gemini provider converts a canonical `role=tool` message into a Gemini
`functionResponse` part, packing the tool output under a `{"result": ...}` object
(`canonicalToGemini` in `plugins/providers/google/google.go`).

## The bug (HIGH)
```go
resp := json.RawMessage(`{"result":` + strconv.Quote(m.Content) + `}`)
```

`strconv.Quote` is a **Go string-literal** quoter, not a JSON encoder. For any byte
that JSON must escape as `\uXXXX` — every control byte except `\t \n \r` — it emits
a Go-only escape such as `\x1b`, which is **invalid JSON**. `\x1b` (ESC) is extremely
common: any tool output carrying ANSI/terminal color codes contains it; so do binary
greps, logs with embedded NULs, etc.

Impact: `encodeRequest`'s `json.Marshal` of the wire struct validates the embedded
`json.RawMessage` and fails (`invalid character 'x' in string escape code`), so
`Provider.Complete` returns an encode error. The agent loop cannot progress past
that tool result on Gemini — every subsequent turn re-sends the poisoned history and
re-fails. Only Gemini is affected (Anthropic/OpenAI/Cohere put tool content in a
normal struct string field that `encoding/json` escapes correctly). Severity HIGH —
a common, non-edge tool output (color codes) wedges the whole run on Google.

## The fix
Let `encoding/json` quote the content:

```go
quoted, err := json.Marshal(m.Content)
if err != nil { return nil, fmt.Errorf("google: encode tool result: %w", err) }
resp := json.RawMessage(`{"result":` + string(quoted) + `}`)
```

`json.Marshal` of a string emits a valid JSON string with correct `\uXXXX` escaping.
The `{"result": ...}` shape is unchanged.

## Test + negative control
`plugins/providers/google/tool_result_internal_test.go` (white-box):
`TestEncodeRequest_ToolResultControlBytesValidJSON` — encodes a request whose tool
result contains `\x1b[31m…\x1b[0m` (ANSI) and a NUL, and asserts `encodeRequest`
succeeds and `json.Valid(body)`.

**Negative control:** restoring `strconv.Quote` made `encodeRequest` fail with
`json: error calling MarshalJSON for type json.RawMessage: invalid character 'x' in
string escape code` — the exact wedge. Restored; test passes.

## Provenance
From the scoped review of provider request/decode internals (anthropic, openai,
cohere reviewed CLEAN for auth/decode/tool-call/token handling; only this hand-rolled
JSON in google was broken). A separate LOW (event.go `json.RawMessage` payload
aliasing footgun) is noted as a follow-up.

## Verification / gate
- `plugins/providers/google` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
