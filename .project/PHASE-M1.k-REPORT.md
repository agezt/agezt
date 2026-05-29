# Phase Report — Milestone 1.k (Cohere v2 chat adapter)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-09](TASKS.md). Continues
> [PHASE-M1.j-REPORT.md](PHASE-M1.j-REPORT.md).

## Scope

M1.k adds Cohere via the v2 `/v2/chat` API. The wire is close to
OpenAI on the *request* side (Bearer auth, `messages[]` with roles,
tools as functions, tool results via `role:"tool"` + `tool_call_id`)
but distinctly different on the *response* side — `message.content`
is a typed-block array (`[{type:"text", text}]`, Anthropic-style)
rather than a single string, and usage is nested under
`usage.tokens.{input,output}`. That's enough divergence to deserve
its own adapter rather than another openai fold-in.

| | M1.j | M1.k | Δ |
|---|---:|---:|---:|
| Supported catalog providers | 123 | **124** | +1 (cohere) |
| Families wired | 6 | **7** | +1 (cohere) |
| Wire adapter packages | 4 | **5** | +1 (cohere) |

| Concern | M1.k status |
|---|---|
| Cohere v2 `/v2/chat` adapter (Bearer auth) | ✅ `plugins/providers/cohere` |
| Default base URL (`api.cohere.com`) when catalog `api` empty | ✅ via `defaultBaseURL` |
| Response `message.content` as **string or array of typed blocks** | ✅ tolerated both shapes |
| Nested usage (`usage.tokens.{input,output}_tokens`) | ✅ |
| `finish_reason` mapping (COMPLETE / STOP_SEQUENCE / MAX_TOKENS / TOOL_CALL) | ✅ |
| Tool calls: openai-shaped (`tool_calls[]` with JSON-string `arguments`) | ✅ |
| Tool results: `role:"tool"` + `tool_call_id` round-trip | ✅ |
| Synthetic stable IDs when Cohere omits `tool_calls[].id` | ✅ `call-<i>` fallback |
| Vertex / Bedrock / Azure | ⏳ M1.l+ |

## Changes

### 1. New `plugins/providers/cohere` package

In-process Cohere v2 Provider. Same Provider shape as the other wire
packages: `APIKey`, `Endpoint`, `BaseURL`, `Model`, `HTTP`. `Name()`
returns `"cohere"` (overridden to the catalog id by `namedProvider`
in compat).

**resolveEndpoint** precedence:

```
1. explicit Endpoint
2. BaseURL — append /chat if already ends with /v2; else /v2/chat
3. DefaultBaseURL (https://api.cohere.com) + /v2/chat
```

**Dialect translation**:

- Canonical `system` folded into the first message with role
  `"system"` (Cohere accepts it inline, not as a separate field).
- `RoleUser`/`RoleAssistant`/`RoleTool` map 1:1 to Cohere roles;
  `RoleAssistant` carries `tool_calls[]` with `arguments` as a
  JSON-encoded string (openai convention).
- `RoleTool` requires `tool_call_id`; folded into a Cohere
  `tool`-role message.
- Response decoding tolerates `message.content` as **either** a
  plain string (older variants) **or** a `[{type:"text", text}]`
  array (modern v2). One test per shape:
  `TestComplete_TextResponseAsBlocks` and
  `TestComplete_TextResponseAsString`.
- `finish_reason`: `COMPLETE`/`STOP_SEQUENCE`/empty → `StopEndTurn`;
  `MAX_TOKENS` → `StopMaxTokens`; `TOOL_CALL` → `StopToolUse`. Plus
  the defensive "tool_calls present → StopToolUse regardless of
  finish_reason" rule, identical to the openai adapter.
- Per-call IDs synthesized as `call-<i>` when missing — Cohere
  sometimes omits them on streamed deltas leaking into batch
  responses (SPEC-15: canonical `ToolCall.ID` must be non-empty).
- Nested usage (`cr.Usage.Tokens.{Input,Output}Tokens`) carried into
  canonical `agent.Usage`.

7 tests in `plugins/providers/cohere/cohere_test.go`: blocks-content,
string-content, tool calls with IDs, 3-leg tool-result round-trip,
3-case endpoint resolution, no-key, API error.

### 2. `compat.Build` routes FamilyCohere to the new adapter

```go
case catalog.FamilyCohere:
    cp := cohere.New(apiKey)
    cp.BaseURL = base
    cp.Endpoint = ""
    cp.Model = modelID
    return &namedProvider{name: p.ID, inner: cp}, modelID, nil
```

`defaultBaseURL` extended:

```go
case catalog.FamilyCohere:
    return "https://api.cohere.com"
```

`IsSupportedFamily` extended; `TestIsSupportedFamily` table now lists
`FamilyCohere: true`. The unsupported-family test was reworked to
use AWS Bedrock as the canonical "still-deferred" entry (it's the
next big lift — SigV4 + regional URL building).

### 3. Refusal message reflects the new frontier

```
M1.k supports anthropic + ollama + openai + openai-compatible
              + google + mistral + cohere;
vertex/bedrock/azure land in M1.l+
```

## Demo transcript

Reuses the demo home from prior phases.

### Step 1 — Cohere is now classified

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list | grep "^  cohere"
  cohere  (Cohere, family=cohere)  [no creds]
```

### Step 2 — daemon banner routes through the Cohere adapter

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=cohere \
  COHERE_API_KEY=co-fake AGEZT_MODEL=command-r-plus-08-2024 agezt
  governor : primary=cohere(catalog; family=cohere,
             model=command-r-plus-08-2024) → fallback=mock(offline),
             daily_ceiling=$20.00
```

models.dev leaves `api` empty for Cohere — same pattern as
Anthropic/OpenAI/Mistral. The default URL (`api.cohere.com`) from
`defaultBaseURL` fills the gap.

### Step 3 — Bedrock still refused with the M1.l hint

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=amazon-bedrock \
  AWS_ACCESS_KEY_ID=fake AWS_SECRET_ACCESS_KEY=fake AWS_REGION=us-east-1 agezt
agezt: compat: provider family not yet supported:
  family="aws-bedrock" provider="amazon-bedrock"
  (M1.k supports anthropic + ollama + openai + openai-compatible
   + google + mistral + cohere;
   vertex/bedrock/azure land in M1.l+)
```

## Architectural consequences

1. **Cohere proves the adapter axis is the right unit.** Pre-M1.k,
   the temptation was to treat Cohere as "almost openai" and bolt it
   on as another openai-compatible variant. The content-as-blocks
   response shape would have leaked into the openai adapter's
   decoder and forced it to learn a per-vendor conditional. Keeping
   the adapter axis tied to wire-shape *response* differences (not
   just request differences) keeps each wire package single-purpose.

2. **Permissive decoding pays off for evolving APIs.** Cohere is
   actively evolving v2 — some endpoints already return
   string-content, others return block-content. The
   `json.RawMessage` + "try string, then blocks" pattern means the
   adapter survives format drift without a version flag.

3. **The unsupported-family test now picks the hardest remaining
   case.** Bedrock is the canonical "this needs real work"
   placeholder — once it lands in M1.l/m, the test target rotates to
   Vertex or Azure. This is a small but useful pattern: the
   refused-family test is implicit documentation of *which family is
   the current frontier*.

## Deferrals → M1.l and beyond

Still returning `ErrFamilyUnsupported` after M1.k:

- **azure** (`@ai-sdk/azure`) — openai-shaped body but resource-
  specific URL (`https://{resource}.openai.azure.com/openai/deployments/
  {deployment}/chat/completions?api-version=2024-02-15-preview`) and
  `api-key` header (not Bearer). Likely a thin wrapper around the
  openai adapter with a URL builder.
- **aws-bedrock** (`@ai-sdk/amazon-bedrock`) — SigV4 signing,
  regional URL, model-id-in-path. Bearer-token-mode (`AWS_BEARER_TOKEN_BEDROCK`)
  is simpler if available.
- **google-vertex** / **google-vertex/anthropic**
  (`@ai-sdk/google-vertex`, `…/anthropic`) — service-account OAuth
  via `google.golang.org/api/option`, regional URL builder.

Unchanged deferrals from prior milestones: subscription-first
routing, `agt provider creds`, browser tool, plugin host, Pulse v1,
planner.

## Files touched

```
plugins/providers/cohere/cohere.go         NEW (~310 LoC)
plugins/providers/cohere/cohere_test.go    NEW (7 tests, 3 sub-tests)
plugins/providers/compat/compat.go         (+ cohere import + FamilyCohere case
                                              + defaultBaseURL entry
                                              + IsSupportedFamily entry
                                              + doc + error-message update)
plugins/providers/compat/compat_test.go    (+ TestBuild_CohereFamilyRoutesToCohereWire
                                              + FamilyCohere: true in enumeration
                                              + reworked unsupported-family test
                                                  to use AWS Bedrock)
```

No schema changes. No daemon-command changes.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 256 pass, 0 fail (up from 245 in M1.j)
```
