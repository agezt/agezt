# M482 — Event: copy a json.RawMessage payload instead of aliasing the caller's bytes

## Context
`event.New` computes the BLAKE3 chain hash over the event's canonical bytes
(including its `Payload`). `marshalPayload` prepares the payload from `Spec.Payload`.

## The bug (LOW)
```go
func marshalPayload(p any) (json.RawMessage, error) {
    if p == nil { return nil, nil }
    if rm, ok := p.(json.RawMessage); ok {
        return rm, nil   // aliases the caller's backing array
    }
    return json.Marshal(p)
}
```

When a caller passes a `json.RawMessage`, the event's stored `Payload` shares
backing storage with the caller's slice. `New` has already computed `Hash` over
those bytes; if the caller later mutates that slice, `e.Payload` silently diverges
from `e.Hash`, and `VerifyHash` (used for journal integrity / tamper detection)
fails. Requires caller misuse, hence LOW — but it's a real integrity footgun in the
event-sourcing core.

## The fix
Copy the bytes:

```go
if rm, ok := p.(json.RawMessage); ok {
    return append(json.RawMessage(nil), rm...), nil
}
```

## Test + negative control
`kernel/event/event_test.go`: `TestNew_CopiesRawMessagePayload` — builds an event
from a `json.RawMessage` payload, mutates the caller's slice afterward, and asserts
`e.Payload` is unchanged and `VerifyHash` still passes.

**Negative control:** restoring `return rm, nil` made `e.Payload` become `XXXXXXXXX`
after the caller's mutation and `VerifyHash` fail (`invalid character 'X' …`).
Restored; test passes.

## Provenance
The LOW follow-up from the kernel/artifact + kernel/event review (artifact, event
kinds, and the rest of event — hash chain, canonical determinism, decode — reviewed
CLEAN).

## Verification / gate
- `kernel/event` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
