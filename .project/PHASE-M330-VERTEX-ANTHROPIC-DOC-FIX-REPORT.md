# M330 — Fix stale "Anthropic-on-Vertex unimplemented" docs + lock-in test

## Why
Two comments in `plugins/providers/compat/compat.go` (the package doc and the
`FamilyGoogleVertex` case) stated that Anthropic-on-Vertex (`claude-*` models via
the `:rawPredict` endpoint) "remains unimplemented." That is false: the vertex
adapter has implemented it for some time (`vertex/anthropic.go` —
`completeAnthropic` / `completeStreamAnthropic`, extended with extended thinking
in M321), and `vertex.Provider.Complete` dispatches on the model id, so a
`claude-*` model under the `google-vertex` family already routes to the Anthropic
Messages body. A maintainer or operator reading those comments would wrongly
conclude Claude-on-Vertex isn't supported. Stale capability docs are a real
correctness issue in a codebase that relies on its comments being accurate.

## What
- **`plugins/providers/compat/compat.go`**: corrected both comments to state that
  Anthropic-on-Vertex IS supported — the vertex adapter dispatches on the model
  id, so a `claude-*` model under `google-vertex` routes to the Anthropic Messages
  body on `:rawPredict` (no separate family). (External/federated ADC genuinely
  remains unimplemented — that part of the doc was accurate and is kept.)
- No behaviour change — the capability already worked; only the documentation was
  wrong.

## Verification
- **`plugins/providers/compat/compat_test.go`**
  `TestBuild_VertexFamilyRoutesClaudeToAnthropicWire` (new): builds the
  `google-vertex` family with a `claude-opus-4-7@20251031` model id and a mock
  OAuth + API server, then `Complete`s. Asserts the request hit the **anthropic
  publisher `:rawPredict`** path (not Gemini's `:generateContent`), carried the
  `anthropic_version` Messages-body field, used the minted Bearer token, and the
  Anthropic-shaped response decoded correctly. This locks in the
  previously-untested-through-compat capability so it can't silently regress.
- Full suite **2023** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged. No CHANGELOG
  entry — no user-facing behaviour changed (docs + test only).

## Scope notes
- Found while sweeping the codebase for stale `TODO` / `not yet` / `unimplemented`
  markers after the Bedrock vendor work.
- The capability has existed since the Anthropic-on-Vertex adapter shipped; M321
  added extended-thinking on top. This milestone just makes the docs honest and
  pins the behaviour with a test.
