# M483 — Vertex tool-result JSON built with strconv.Quote → invalid JSON (HIGH)

## Context
The Vertex AI provider uses the same Gemini wire format as the Google provider and
packs a `role=tool` message's output under `{"result": ...}` in
`canonicalToVertex` (`plugins/providers/vertex/vertex.go`).

## The bug (HIGH) — identical to M481
```go
resp := json.RawMessage(`{"result":` + strconv.Quote(m.Content) + `}`)
```

Same defect M481 fixed in google.go: `strconv.Quote` is a Go string-literal quoter,
so a control byte (the ANSI escape `\x1b` common in tool output, NUL, etc.) becomes
a Go-only `\xNN` escape that is invalid JSON. `encodeRequest`'s `json.Marshal`
validates the embedded `json.RawMessage` and fails, so `Provider.Complete` returns
an encode error and the agent loop wedges on Vertex for any such tool result. M481's
review scope explicitly excluded vertex.go, so the twin was missed; a tree-wide scan
for `strconv.Quote` (prompted by M481) found it.

## The fix
Same as M481 — encode the content with `encoding/json`:

```go
quoted, err := json.Marshal(m.Content)
if err != nil { return nil, fmt.Errorf("vertex: encode tool result: %w", err) }
resp := json.RawMessage(`{"result":` + string(quoted) + `}`)
```

## Test + negative control
`plugins/providers/vertex/tool_result_internal_test.go` (white-box):
`TestEncodeRequest_ToolResultControlBytesValidJSON` — a tool result with `\x1b`
(ANSI) and NUL must encode to valid JSON.

**Negative control:** restoring `strconv.Quote` reproduced `invalid character 'x' in
string escape code`. Restored; test passes.

## Provenance
Found by a tree-wide `strconv.Quote`-into-JSON scan triggered by M481 (the same-class
Gemini bug). The scan confirmed these two were the only hand-rolled-JSON sites; no
`fmt.Sprintf`-built JSON bodies exist elsewhere.

## Verification / gate
- `plugins/providers/vertex` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
