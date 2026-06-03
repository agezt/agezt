# M221 — Go plugin authoring SDK (`plugins/sdk`)

## Why
v1.0 (M8) closed the *kernel* side of the federated vision. The remaining vision
gap, called out in ROADMAP §5, is the **ecosystem**: "the plugin architecture +
polyglot SDK means the community can build the capability army." Today that army
is gated by friction — writing a plugin means hand-implementing the
line-delimited JSON protocol. The reference plugin
(`kernel/plugin/testdata/echoplugin`) is **~260 lines** of pure protocol
plumbing: the stdin read loop, the request/response/callback frame demux,
write serialisation across goroutines, progress emission, and host-callback
routing. None of that is the plugin's actual job.

This milestone removes that friction for Go authors — the first concrete step of
the polyglot-SDK story — without touching the kernel or the wire contract.

## What
A new stdlib-only package, `plugins/sdk`, that collapses the protocol to:

```go
func main() {
    sdk.Serve(sdk.Tool{
        Name: "greet", Description: "...", InputSchema: schema,
        Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
            return sdk.Text("hello"), nil
        },
    })
}
```

`Serve` (and `ServeRW` for tests/embedding) handles, on the author's behalf:
- **initialize** → replies with the registered tool definitions (open-object
  schema default when none given).
- **tool/invoke** → routes to the matching `Handle` on its own goroutine, so the
  read loop stays responsive to concurrent invokes and to callback replies.
- **shutdown / EOF / ctx-cancel** → clean return; in-flight handlers are awaited.
- **Progress** via `Emit(ctx, msg)` — keyed to the in-flight request id, so the
  author never touches a frame id.
- **Host callbacks** via `CallHost(ctx, tool, input)` — the `host/invoke`
  direction, with the reply demuxed back to the blocked handler.
- **Panic containment** — a panicking handler returns a tool-level error
  (`IsError`), it does not crash the plugin (mirrors the kernel agent panic
  firewall, M168).
- **Goroutine-safe writes** — every frame (response, progress, callback request)
  is serialised under one mutex so bytes never interleave on shared stdout.

Helpers: `Text(s)`, `Errorf(...)`. Registration is validated (name + handler
required; duplicate names rejected).

**Dependency discipline.** The package imports ONLY the Go standard library and
deliberately does **not** import `kernel/plugin` or `kernel/agent` — a plugin
must never have to compile against the daemon (DECISIONS B0). The wire types are
independent copies of the small plugin-side contract in
`kernel/plugin/protocol.go`.

## Files
- `plugins/sdk/sdk.go` — the SDK (new).
- `plugins/sdk/sdk_test.go` — 16 unit tests over `ServeRW` via in-memory pipes:
  initialize listing, invoke success, handler-error→IsError mapping, `Errorf`,
  unknown tool, bad params, unknown method, panic containment + survival,
  progress ordering, `Emit`/`CallHost` outside-handler no-op/error, host-callback
  round-trip + host-error surfacing, bad registration, EOF, ctx-cancel.
- `plugins/sdk/integration_test.go` — 4 live tests: compiles the example plugin
  to a real binary and drives it through the **actual kernel plugin host**
  (`plugin.Spawn`): tool registration, invoke success/error, progress
  streaming (`InvokeWithProgress`), and a host callback wired via
  `Config.HostTools`.
- `plugins/sdk/example/greet/main.go` — a complete runnable SDK plugin
  (`greet` / `slow` / `shout`), doubling as the author template.

## Verification
- `go test ./plugins/sdk/...` — 20 tests pass (16 unit + 4 integration).
- Full suite: `go test ./...` — green; **1693 → 1713** tests, **64 → 66**
  packages (`plugins/sdk` + `plugins/sdk/example/greet`).
- `gofmt -l` (CRLF-normalised) clean on all four new files.
- `go vet ./plugins/sdk/...` — clean.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- **Live proof:** the integration test is the proof — an SDK-authored binary,
  compiled and spawned by the real host, exercised end-to-end over OS pipes. If
  the SDK's wire behaviour drifted from the host's expectations, it fails.

## Scope notes
- This is the **Go** SDK. The polyglot promise (ts/py/rust) and a
  `create-agezt-plugin` scaffolder remain future milestones; the wire contract
  they target is unchanged and already documented.
- No kernel, contract, or existing-plugin changes — purely additive.
