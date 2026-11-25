# W5.14 — the agent gateway's surface gets a contract and a door

## 1. What was wrong

The **agent gateway** is the daemon's door to agent subprocess code: a local
socket (or a loopback port) that a subprocess uses to publish events, write
memory, write logs, mint a token and read the config center.

Its **16 routes** lived as string literals in `kernel/agentgw/gateway.go`, and
**five** of them take a request body. **None** of those five said what the
body looks like: four of them were read into an **anonymous struct** and one
(`ConfigSetRequest`) into a named struct the contract could not see. So what
happened to a mistyped body was decided by Go's `encoding/json` — and the
answer named a Go struct:

| the body | the old answer |
|---|---|
| `POST /v1/eventbus/publish {"event":42,…}` | 400 `INVALID_REQUEST` — *"json: cannot unmarshal number into Go struct field .event of type string"* |
| `POST /v1/eventbus/publish {"event":null,…}` | **500** `PUBLISH_ERROR` — *"event: subject required"* (the null became `""`) |
| `POST /v1/eventbus/publish {"event":"x","payload":"not-an-object"}` | 400 — *"…Go struct field .payload of type map[string]interface {}"* |
| `POST /v1/eventbus/publish []` | 400 — *"json: cannot unmarshal array into Go value of type struct { Event string \"json:\\\"event\\\"\"; Payload map[string]interface {} … }"* |
| `POST /v1/memory/write {"type":42,…}` | 400 — *"…Go struct field .type of type string"* |
| `POST /v1/memory/write {"content":null}` | **500** `WRITE_ERROR` — *"memory: empty content"* (the null became `""`) |
| `POST /v1/log/write {"level":42,…}` | 400 — *"…Go struct field .level of type string"* |
| `POST /v1/log/write {"message":null}` | **202** `{"status":"logged"}` — the null was accepted and an empty message was logged |
| `POST /v1/token/create {"caps":"all"}` | 400 — *"…Go struct field .caps of type []string"* |
| `POST /v1/token/create {"max_rpm":"60"}` | 400 — *"…Go struct field .max_rpm of type int"* |

Three things were wrong with that, and only one of them is the sentence:

1. **The sentence was Go's, not the registry's.** A caller of the gateway
   learned a different language from a caller of `/api/v1` — and the answer
   leaked the handler's **private struct definition** into the wire (the `[]`
   case above prints the anonymous struct's source text).
2. **A null was not refused at all.** It became the zero value, and the zero
   value then failed somewhere else: **two** of the measured nulls answered a
   **500** (a caller's null is not a server error), and one was **accepted**.
3. **The surface had no contract at all.** Nothing named its paths, its
   methods, its capabilities or its bodies, so nothing could check them.

## 2. What was built

### 2.1 The surface has one home

`kernel/contract/agentgwapi` holds the gateway's route table — the same shape
as the console's table (`kernel/contract/opapi`) and the REST surface's
(`kernel/contract/restapiapi`):

```go
type Route struct {
	ID         string
	Method     string
	Path       string
	Capability string
	Args       []string
	Body       json.RawMessage // the JSON Schema of the request body
	Config     bool            // the config center serves it
	Public     bool            // it needs no token
}
```

The **16 routes** are named there — path, method, the capability a caller
needs, the query arguments the route reads, and the body it takes — and the
gateway registers **from** the table:

```go
func (g *Gateway) Handler() http.Handler { … }
```

The drift is caught at boot, not at the call site: a route in the contract
without a handler — or a handler without a route — panics when the gateway
builds its handler, exactly as the REST surface does (W5.2).

### 2.2 The bodies are named, and the door checks them

The five bodies are JSON Schemas in the contract, and they follow the
registry's three rules: **types, not presence** (no `required` list — a
missing key is still the handler's *"key is required"*);
**`additionalProperties: true`** (a body is the stable subset of what the
daemon reads); and the **registry's type words** (an integer is a `number`).

The gateway's door is the transport's shared core
(`kernel/httpserver.DecodeJSONBody`), so the check is the same everywhere, and
the gateway keeps its own **voice** — the envelope its subprocesses already
read:

```json
{"error":{"code":"INVALID_REQUEST","message":"operation eventbus_publish input: $.event has wrong type: got number, want string"}}
```

### 2.3 The bodies are typed

The four anonymous structs are **named** now (`eventPublishRequest`,
`memoryWriteRequest`, `logWriteRequest`, `tokenCreateRequest`), so the
handler's private shape is a thing the contract and the daemon can agree on
instead of a literal in a decoder call.

## 3. The measurement

| | before | after |
|---|---|---|
| routes on the gateway's surface | 16 (string literals in the handler) | 16 (**named in the contract**) |
| routes registered from the contract | **0** | **16** |
| bodies the contract names a shape for | **0** | **5** |
| endpoints behind a door | **0** | **5** |
| request fields checked before the handler reads them | **0** | **22** |
| bodies refused with the registry's sentence | **0** | **22** |
| nulls refused instead of silently zero | **0** | **22** |
| the measured nulls that answered a **500** | **2** | **0** |
| the null that was **accepted** | **1** | **0** |
| the daemon's own body structs | 1 named + **4 anonymous** | **5 named** |

The 22 fields are `eventbus_publish` (3), `memory_write` (4), `log_write` (3),
`token_create` (5) and `config_set` (7).

### 3.1 What did not change

- **The valid bodies.** A publish still answers 202, a memory write still 201
  with its record, a log write still 202, a token mint still 201.
- **The gateway's own refusals.** A missing key is still `MISSING_KEY`, a
  missing capability is still `FORBIDDEN`, an unknown capability is still
  `INVALID_CAPABILITY`, and the ACL fields of a config write are still
  refused with **403** — the security rule is untouched.
- **The status codes.** Every body refusal is still a **400** with the
  `INVALID_REQUEST` code.
- **The route set.** The same sixteen paths, the same methods, the same
  capabilities.

### 3.2 The two refusals that did change

| the body | before | after |
|---|---|---|
| a body over the cap | 400 `INVALID_REQUEST` — *"http: request body too large"* | 400 `INVALID_REQUEST` — the same words |
| a malformed body (the config route) | 400 `INVALID_REQUEST` — *"invalid JSON: …"* | 400 `INVALID_REQUEST` — the decoder's words, without the prefix |

The cap's message is unchanged. The config route's malformed-body answer lost
its private *"invalid JSON: "* prefix, so the gateway now has **one** voice
for a body it cannot read — the other four routes always answered that way.

## 4. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/agentgwapi`: `TestTheRouteTableIsWellFormed` | every route has an id, a method and a path, and no two claim the same id or the same method+path |
| 2 | `kernel/contract/agentgwapi`: `TestEveryBodySchemaIsASchemaTheDoorCanCheck` | every body schema parses, is an object, carries no `required` list, allows an undeclared field, and names every field with a type |
| 3 | `kernel/contract/agentgwapi`: `TestTheSchemaLookupAnswersTheTable` | the lookup the door uses answers the schema the table states — and answers none for a route that takes no body |
| 4 | `kernel/agentgw`: `TestTheContractAndTheDaemonNameTheSameBody` | the contract's schema and the daemon's own struct name the **same fields with the same types**, in both directions |
| 5 | `kernel/agentgw`: `TestTheDoorRefusesAMistypedField`, `TestTheDoorRefusesANullField` | a mistyped field — and a null — is refused with the registry's words, for every field of every body |
| 6 | `kernel/agentgw`: `TestTheDoorLetsAContractBodyThrough`, `TestTheDoorAllowsAFieldTheContractDoesNotName`, `TestTheDoorRefusesABodyThatIsNotAnObject` | the door does not close the gateway, an undeclared field stays allowed, and a body that is not an object is refused |
| 7 | `kernel/agentgw`: `TestTheGatewayKeepsItsOwnRefusals` | a missing key is still the gateway's `MISSING_KEY`, and a refused body is still the gateway's envelope — with the registry's words inside it |

Guard 4 states its expectation from the **daemon's own struct**, not from the
contract's schema: a schema that mistyped a field would then agree with itself
and pass. That is the same trap the W5.12 slice found.

## 5. The mutations

Five mutations, five caught, all reverted (`.temp_files/w514_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the door measures the wrong route's schema | *"the "memory_write" route: a mistyped "type" said "json: cannot unmarshal number into Go struct field memoryWriteRequest.type of type string", want it to contain "operation memory_write input: $.type has wrong type: got …""* |
| M2 | the contract mistypes the memory write body | *"the contract's "memory_write" schema names "content" as "number", the daemon reads it as "string""* |
| M3 | the refusal drops the registry's wrapper | *"the "eventbus_publish" route: a mistyped "event" said "$.event has wrong type: got number, want string", want it to contain "operation eventbus_publish input: …""* |
| M4 | the contract's schema stops allowing an undeclared field | *"the "eventbus_publish" route refused a body carrying an undeclared field: … $.extra_field is not allowed by schema"* |
| M5 | the contract's schema is malformed | *"the "token_create" route's schema is not readable JSON: unexpected end of JSON input"* |

**M4 escaped the first run**, and the escape was the point: the guard looked
at the **status code**, and a route may answer 400 for its own reasons (the
token route refuses an unknown capability with `INVALID_CAPABILITY`) — so a
strict status check would have been wrong twice. The guard now looks for the
registry's **words** ("is not allowed by schema"), and M4 fires.

M5 is the mutation that proves the contract's own guard bites: a schema with
one brace missing would have made the door refuse **every** body, and the
contract's test says so before the daemon sees it.

## 6. Preserved behaviour

- **The wire protocol did not change.** A body that fits the contract reaches
  the handler exactly as before.
- **The gateway's surface did not change.** The same sixteen paths, methods and
  capabilities; only the table that names them moved into the contract.
- **The security rules did not change.** The config route still refuses the
  ACL fields with **403**, and the token route still refuses a capability
  the parent does not hold.
- **The SDKs did not change.** The gateway is not part of the SDK contract: the
  SDKs speak `/api/v1`, and the callers of this surface are the daemon's own
  agents.

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
- Python SDK: `python -m unittest discover -s tests` — green.
- TypeScript SDK: `npm test` (tsc + node:test) — green.
- Rust SDK: `cargo fmt --check` + `cargo test` — clean.

Measured: bodies the contract names a shape for **0 → 5**; endpoints behind a
door **0 → 5**; request fields checked before the handler reads them
**0 → 22**; bodies refused with the registry's sentence **0 → 22**; nulls
refused **0 → 22**; the measured nulls that answered a 500 **2 → 0**; routes
registered from the contract **0 → 16**.
