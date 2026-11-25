# W5.15 — the OpenAI-compatible surface gets a contract and a door

## 1. What was wrong

The **OpenAI-compatible surface** is the daemon's door to any OpenAI client,
SDK or IDE: it speaks OpenAI's own wire protocol so a caller can drive Agezt
as if it were OpenAI. Its **5 routes** were registered by hand in
`kernel/openaiapi/openaiapi_server.go`, and **two** of them take a JSON body —
`POST /v1/chat/completions` and `POST /v1/responses`.

The two bodies were read into **typed** structs (`chatRequest`,
`responsesRequest`) — that part was already right — but **no contract named
them and no door measured them**. So what happened to a mistyped body was
decided by Go's `encoding/json`, and the answer named a Go struct:

| the body | the old answer |
|---|---|
| `POST /v1/chat/completions {"model":42,…}` | 400 `invalid_request_error` — *"invalid JSON body: json: cannot unmarshal number into Go struct field chatRequest.model of type string"* |
| `POST /v1/chat/completions {"messages":null}` | 400 `invalid_request_error` — *"no usable message content"* (the null became an empty slice) |
| `POST /v1/chat/completions []` | 400 — *"…Go value of type openaiapi.chatRequest"* (the private struct leaks into the wire) |
| `POST /v1/chat/completions {"stream":"yes"}` | 400 — *"…Go struct field chatRequest.stream of type bool"* |
| `POST /v1/responses {"model":42,…}` | 400 — *"…Go struct field responsesRequest.model of type string"* |
| `POST /v1/responses {"input":null}` | 400 — *"no usable input content"* |

Three things were wrong with that, and only one of them is the sentence:

1. **The sentence was Go's, not the registry's.** A caller of this surface
   learned a different language from a caller of `/api/v1` or of the agent
   gateway — and the answer leaked the handler's **private struct name** into
   the wire.
2. **A null was not refused at all.** It became the zero value, and the request
   went on. In the measured set, **15 of the 18** nulls were **accepted**:
   `{"model":null,…}` was a run against the default model,
   `{"stream_options":null,…}` was a run without usage, `{"text":null,…}` was a
   plain run. The other three were refused — but by the handler, in the
   surface's own words, not in the registry's.
3. **The surface had no contract at all.** Nothing named its paths, its methods
   or its bodies, so nothing could check them.

## 2. What was built

### 2.1 The surface has one home

`kernel/contract/openaiapi` holds the surface's route table — the same shape as
the gateway's table (`kernel/contract/agentgwapi`) and the REST surface's
(`kernel/contract/restapiapi`):

```go
type Route struct {
	ID        string
	Method    string
	Path      string
	Body      json.RawMessage // the JSON Schema of the request body
	Multipart bool            // the body is an upload, not JSON
}
```

The **5 routes** are named there — path, method and the body each route takes
— and the server registers **from** the table:

```go
func (s *Server) Handler() http.Handler { … }
```

The drift is caught at boot, not at the call site: a route in the contract
without a handler — a handler without a route — or a contract method that
disagrees with the transport policy panics when the server builds its handler,
exactly as the gateway does (W5.14) and the REST surface does (W5.2).

### 2.2 The bodies are schemas, and the door checks them

The two bodies are JSON Schemas in the contract, and they follow the registry's
three rules: **types, not presence** (no `required` list — a missing model is
still the default model); **`additionalProperties: true`** (a body is the
stable subset of what the daemon reads, and this surface is OpenAI's own wire
protocol, so a field Agezt does not read may still be one a client sends); and
the **registry's type words** (an integer is a `number`).

**One union, and it is the vendor's.** A chat message's `content` is either
the text itself or an array of typed parts (an image part among them), and the
Responses `input` is either the text itself or an array of message items — the
daemon reads both in exactly those two shapes, so the schema names both
(`"string|array"`). Everywhere else the type is single.

The surface's door is the transport's shared core
(`kernel/httpserver.DecodeJSONBody`), so the check is the same everywhere, and
the surface keeps its own **voice** — the OpenAI error envelope every OpenAI
client already reads:

```json
{"error":{"message":"operation chat_completions input: $.model has wrong type: got number, want string","type":"invalid_request_error"}}
```

### 2.3 The daemon's own decoder is gone

`decodeBody` (the surface's private `json.NewDecoder` reader) is **deleted**:
both body-taking handlers now go through the door, so there is exactly one
place where a request body becomes a value on this surface.

## 3. The measurement

Measured on the tree at `origin/main` (`11caf581`) with the same probe that
measured the "after" side: **18 fields** across the two bodies, each probed
twice (a wrong type and a null), plus the root.

| | before | after |
|---|---|---|
| routes on the surface | 5 (registered by hand) | 5 (**named in the contract**) |
| routes registered from the contract | **0** | **5** |
| bodies the contract names a shape for | **0** | **2** |
| endpoints behind a door | **0** | **2** |
| request fields checked before the handler reads them | **0** | **18** |
| probes refused with the registry's sentence (18 wrong + 18 null) | **0** | **36** |
| nulls refused instead of silently zero | **3** | **18** |
| nulls **accepted** (a 200) | **15** | **0** |
| the answers that named a private Go struct | **18** | **0** |
| the surface's own decoder | 1 (`decodeBody`) | **0** |

The 18 fields are `chat_completions` (9: `model`, `messages`, `messages[].role`,
`messages[].content`, `stream`, `stream_options`, `stream_options.include_usage`,
`response_format`, `response_format.type`) and `responses` (9: `model`, `input`,
`instructions`, `stream`, `text`, `text.format`, `text.format.type`,
`response_format`, `response_format.type`).

The before column was measured on a worktree of `origin/main`; the after column
on the branch, with the same probe (`.temp_files/w515probe/main.go`).

### 3.1 What did not change

- **The valid bodies.** A chat completion still answers 200 with the same
  object, a Responses call still answers 200 with the same object, the
  streaming paths still stream, and the models routes still answer the same
  list and the same single model.
- **The surface's own refusals.** A message list with no usable content is
  still *"no usable message content"*, and an empty Responses input is still
  *"no usable input content"*.
- **The status codes.** Every body refusal is still a **400** with the
  `invalid_request_error` type; an oversized body is still a **413**.
- **The route set.** The same five paths, the same methods, the same caps
  (16 MiB for a JSON body, 25 MiB for the audio upload).
- **The multipart route.** `POST /v1/audio/transcriptions` takes an upload, not
  JSON: the contract says so (`Multipart: true`) and the door does not touch
  it.

### 3.2 The refusals that did change

| the body | before | after |
|---|---|---|
| a mistyped field | 400 — *"invalid JSON body: json: cannot unmarshal number into Go struct field chatRequest.model of type string"* | 400 — *"operation chat_completions input: $.model has wrong type: got number, want string"* |
| a null field | 200 (15 of 18), or 400 in the handler's own words | 400 — *"operation chat_completions input: $.model has wrong type: got null, want string"* |
| a body that is not an object | 400 — *"…Go value of type openaiapi.chatRequest"* | 400 — *"operation chat_completions input: $ has wrong type: got array, want object"* |

A **malformed** body keeps the decoder's own words (*"invalid JSON body: EOF"*,
*"invalid JSON body: unexpected end of JSON input"*) and an **empty** body
keeps them too — that is the decoder's error, not a shape error.

## 4. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/openaiapi`: `TestTheRouteTableIsWellFormed` | every route has an id, a method and a path, and no two claim the same id or the same method+path |
| 2 | `kernel/contract/openaiapi`: `TestEveryBodySchemaIsASchemaTheDoorCanCheck` | every body schema parses, is an object, carries no `required` list, allows an undeclared field, and names every property **at every level** with a type |
| 3 | `kernel/contract/openaiapi`: `TestTheSchemaLookupAnswersTheTable` | the lookup the door uses answers the schema the table states — and answers none for a route that takes no body |
| 4 | `kernel/contract/openaiapi`: `TestAnUploadRouteHasNoJSONBody` | a route that takes an **upload** (the audio transcription) has no JSON body, and the lookup answers none for it — a body the door does not measure is never measured as if it were JSON |
| 5 | `kernel/openaiapi`: `TestTheContractAndTheDaemonNameTheSameBody` | the contract's schema and the daemon's own struct name the **same fields with the same types**, in both directions — nested fields included |
| 6 | `kernel/openaiapi`: `TestTheDoorRefusesAMistypedField`, `TestTheDoorRefusesANullField` | a mistyped field — and a null — is refused with the registry's words, for every field of every body |
| 7 | `kernel/openaiapi`: `TestTheDoorLetsAContractBodyThrough`, `TestTheDoorAllowsAFieldTheContractDoesNotName`, `TestTheDoorRefusesABodyThatIsNotAnObject` | the door does not close the surface, an undeclared field stays allowed, and a body that is not an object is refused |
| 8 | `kernel/openaiapi`: `TestTheOpenAISurfaceKeepsItsOwnRefusals` | a message list with no usable content is still the surface's own refusal, and a refused body is still the OpenAI envelope — with the registry's words inside it |

Guard 5 states its expectation from the **daemon's own struct**, not from the
contract's schema: a schema that mistyped a field would then agree with itself
and pass. That is the same trap the W5.13 slice found. It compares the types
as well as the names, and it walks the nested shapes too — a chat message's
`role` and `content`, the streaming options' `include_usage`, the response
format's `type`, the Responses `text.format.type`.

The one union is held by the same ratchet: a `json.RawMessage` is read in
exactly two shapes here (the text itself, or an array of typed parts), and the
ratchet says so out loud rather than letting a schema quietly narrow it.

## 5. The mutations

Six mutations, six caught, all reverted (`.temp_files/w515_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the door measures the wrong route's schema (the chat route measures the responses body) | *"the "chat_completions" route: a mistyped "messages" said "invalid JSON body: json: cannot unmarshal string into Go struct field chatRequest.messages of type []openaiapi.chatMessage", want it to contain "operation chat_completions input: $.messages has wrong type: got ""* |
| M2 | the contract mistypes a field (`model` becomes a number) | *"the contract's "chat_completions" schema names "model" as [number], the daemon reads it as [string]"* |
| M3 | the door stops measuring the body at all | *"the "chat_completions" route: a mistyped "model" said "invalid JSON body: json: cannot unmarshal number into Go struct field chatRequest.model of type string", want it to contain "operation chat_completions input: $.model has wrong type: got ""* |
| M4 | the schema lets a null through (`stream` accepts `"null"`) | *"the "chat_completions" route: a null "stream" answered 200 …"* |
| M5 | the contract's schema stops allowing an undeclared field | *"the "chat_completions" route refused a body carrying an undeclared field: {"error":{"message":"operation chat_completions input: $.extra_field is not allowed by schema","type":"invalid_request_error"}}"* |
| M6 | the contract names a field the daemon does not read (`temperature`) | *"the contract's "chat_completions" schema names "temperature", which the daemon does not read"* |

M1 is the mutation that proves the door measures the **route's own** schema: the
mistyped `model` was still refused (both bodies name it), and the fields only
the chat body has — `messages`, `stream_options`, `response_format` — were not
refused at all, in the Go decoder's words.

M4 is the mutation that proves the null arm is load-bearing: with `"null"` in
the type list, a null `stream` was **accepted** (200) — the exact hole this
slice closes.

## 6. Preserved behaviour

- **The wire protocol did not change.** A body that fits the contract reaches
  the handler exactly as before, and the OpenAI response shapes are untouched.
- **The vendor's names are the vendor's.** The contract names OpenAI's fields
  as OpenAI names them — the shape is the vendor's and it is not changed here;
  what changed is that the daemon now says what it expects of them.
- **The security rules did not change.** Every route is still token-authed, the
  tenant resolver and the tenant authorizer still work as before, and the
  bodies are still read only after the authentication middleware answered.
- **The SDKs did not change.** This surface is not part of the SDK contract: the
  SDKs speak `/api/v1`, and the callers of this surface are OpenAI clients.

## 7. Verification

- `gofmt -l kernel cmd plugins tools sdk` — clean.
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — green.
- `go run ./tools/archcheck` — clean.
- `go run ./tools/deadcodecheck` — clean.
- `go run ./tools/depscheck` — clean.
- `go run ./tools/sdkgen -check` — in sync.
- `go run ./tools/structure-md -check -out .project/STRUCTURE.generated` — in
  sync.
- `go run ./tools/openapi -check` — in sync.
- `go run ./tools/changelog-lint` — clean.
- `go run ./tools/docclaimscheck -base origin/main` — clean.
- `go run ./tools/docsindex` — clean.
- Python SDK: `python -m unittest discover -s tests` — green (51 tests).
- TypeScript SDK: `npm test` (tsc + node:test) — green (31 tests).
- The Rust SDK (`cargo fmt --check` + `cargo test`) was **not** run on this host —
  `cargo` is not installed here — and the CI job `rust-sdk` runs it. Nothing in
  this slice touches the Rust SDK: the OpenAI-compatible surface is not part of
  the SDK contract, and the SDK catalogs did not move (`sdkgen -check` is in
  sync).

Measured: bodies the contract names a shape for **0 → 2**; endpoints behind a
door **0 → 2**; request fields checked before the handler reads them **0 → 18**;
probes refused with the registry's sentence **0 → 36**; nulls refused **3 → 18**;
nulls accepted **15 → 0**; answers naming a private Go struct **18 → 0**;
routes registered from the contract **0 → 5**.
