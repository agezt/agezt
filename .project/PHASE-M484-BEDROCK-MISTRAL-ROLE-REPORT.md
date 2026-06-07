# M484 — Bedrock-Mistral: hard-code the assistant role on decode

## Context
Each Bedrock vendor adapter decodes the provider response into a canonical
`agent.CompletionResponse`. The assistant turn's `Message.Role` must be
`agent.RoleAssistant` so downstream loop bookkeeping / event emission that switches
on the role classifies it correctly.

## The bug (MED)
`decodeMistralOnBedrockResponse` took the role from the wire:

```go
Message: agent.Message{
    Role:    agent.Role(ch.Message.Role),
    Content: ch.Message.Content,
},
```

OpenAI-shaped backends frequently omit `message.role` on the response, so
`ch.Message.Role` is `""` → the canonical role becomes empty (or any other value the
backend sends). Every sibling adapter hard-codes `agent.RoleAssistant`
(anthropic/llama/cohere/ai21/deepseek; Nova explicitly defends against an empty
role). Mistral was the outlier, so a Bedrock-Mistral assistant turn could be
misclassified.

## The fix
Hard-code the role like the siblings:

```go
Role: agent.RoleAssistant,
```

## Test + negative control
`plugins/providers/bedrock/mistral_role_internal_test.go` (white-box):
`TestDecodeMistralOnBedrock_HardcodesAssistantRole` — decodes a response whose
`message` has **no** `role` field and asserts the canonical role is
`agent.RoleAssistant` (and content is preserved).

**Negative control:** restoring `agent.Role(ch.Message.Role)` made the role decode
as `""` — the test FAILED. Restored; test passes.

## Provenance
From the scoped review of the remaining providers (ollama, compat, bedrock
non-binary, internal, sdk). That review confirmed the hand-rolled-JSON anti-pattern
(google/vertex, M481/M483) is **absent** here — every wire body uses typed structs +
`json.Marshal`; internal/httpread, sigv4 shim, bedrock request/auth, nova/ai21/
cohere/llama/deepseek, compat credential resolution, and the SDK framing all
reviewed CLEAN. Two LOW parity notes (ollama empty-media-type data URL; deliberate
max_tokens floor) were judged non-bugs.

## Verification / gate
- `plugins/providers/bedrock` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
