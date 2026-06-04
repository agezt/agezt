# M310 — Ollama honours the run's token cap (MaxTokens → num_predict)

## Why
A provider-parity bug found by mapping `CompletionRequest` fields to what each
provider forwards. `MaxTokens` is enforced by every cloud provider (Anthropic,
OpenAI, Google, Vertex, Bedrock, Cohere) — but the Ollama encoder didn't even
take the field, so an Ollama run **silently ignored the token cap**. The same
output limit that bounds a run on a cloud model was unbounded on a local one.

The tell was already in the code: Ollama's decoder maps a response
`done_reason == "length"` to `StopMaxTokens` — it expected a length stop on the
way back, but the request never sent a limit on the way out.

## What
- **`plugins/providers/ollama/ollama.go`** + **`streaming.go`**: `encodeRequest`
  and `encodeStreamRequest` take a `maxTokens int` and, when it's > 0, set
  Ollama's equivalent — `options.num_predict`. `Complete` and `CompleteStream`
  pass `req.MaxTokens`. 0 omits the option (Ollama's own default — uncapped runs
  are byte-for-byte unchanged).

## Verification
- **`plugins/providers/ollama/ollama_test.go`**:
  `TestEncodeRequest_MaxTokensAsNumPredict` — `MaxTokens=256` →
  `options.num_predict == 256`; `MaxTokens=0` omits `num_predict` entirely. The
  existing role/tool and vision encode tests were updated for the new arg.
- Full suite **1971** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `go.mod` / `go.sum` unchanged. Network-free.

## Scope notes
- Bug fix, not a new feature: brings Ollama in line with the other providers'
  MaxTokens handling. No behaviour change for runs without a cap.
- `CompletionRequest` deliberately exposes no other sampling controls
  (temperature, top_p, stop, seed) — that's a project-wide minimalism choice, not
  a per-provider gap, so it's left alone.
