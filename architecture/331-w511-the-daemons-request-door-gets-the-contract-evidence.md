# W5.11 — the daemon's request door gets the contract

## 1. What was wrong

The `/api/v1` surface — the one the Python, TypeScript and Rust SDKs speak — had a
**request vocabulary with two homes**: the contract's rows
(`kernel/contract/restapiapi`) and the daemon's own private request structs
(`runRequest`, `mailboxSendRequest`, `mailboxAckRequest`).

W5.10 made the two **agree** (a ratchet holds both directions) and made the
SDKs build their bodies from the contract's row. But the daemon still **decoded
its bodies into its own structs**, so what actually happened to a mistyped body
was decided by Go's `encoding/json`, not by the contract.

Measured on the tree at `origin/main` (the old path is
`json.NewDecoder(r.Body).Decode(&req)`):

| body | the old answer |
|---|---|
| `{"intent":42}` | 400 `invalid JSON body: json: cannot unmarshal number into Go struct field runRequest.intent of type string` |
| `{"intent":"x","stream":null}` | **accepted** — `stream` became `false`, and the caller was never told |
| `{"text":"x","help":"yes"}` | 400 `invalid JSON body: json: cannot unmarshal string into Go struct field mailboxSendRequest.help of type bool` |
| `{"text":"x","help":null}` | **accepted** — `help` became `false`, and the caller was never told |
| `{"text":"x","correlation_id":7}` | 400 `invalid JSON body: json: cannot unmarshal number into Go struct field mailboxSendRequest.correlation_id of type string` |
| `{"by":5}` | 400 `invalid JSON body: json: cannot unmarshal number into Go struct field mailboxAckRequest.by of type string` |

Three things were wrong with that:

1. **The words were Go's.** A caller that mistyped a body to the registry got
   `operation workboard_create input: $.max_attempts has wrong type: got
   string, want number`; the same mistake to `/api/v1` got a Go decoder's
   sentence about a Go struct field — a message that names a type the caller
   never sees and a field in a language they did not write.
2. **A null was a silent default.** `encoding/json` accepts `null` for a
   `bool` or a `string` and leaves the zero value. `{"help":null}` was not a
   help request and not a refusal either — it was a message that quietly asked
   for nothing.
3. **The check was bound to the daemon, not to the contract.** The struct said
   what the daemon happened to read *today*. A field the contract declares but
   the daemon ignores (or the other way round) would not have been caught here
   — and a contract that grew a field the daemon did not read would have been
   checked against nothing at all.

## 2. What was built

### 2.1 The contract renders the shape the door measures against

`restapiapi.RequestSchema(endpointID)` renders an endpoint's request row as a
**JSON Schema**:

```json
{"type":"object","additionalProperties":true,"properties":{
  "intent":{"type":"string"},"model":{"type":"string"},"stream":{"type":"boolean"}}}
```

The type mapping speaks the registry's words — an integer is a `number` (the
registry says *"want number"*), a boolean is a `boolean`, a list of strings is
an `array` of `string`.

Two rules keep it the registry's door:

- **No `required` list.** The registry's door checks **types**; presence is the
  handler's business. An absent intent is still `intent is required` from the
  handler, not a schema error.
- **`additionalProperties: true`.** A row is the stable **subset** of a body:
  the daemon may read more than the contract names, and a caller may send more
  than the contract declares.

### 2.2 The door

`kernel/restapi/request_door.go` — `decodeRequestBody` — reads the body,
checks it against the contract's schema with the **same validator the
registry uses** (`kernel/platform/schema`), and then decodes it into the
handler's struct:

```
operation <op> input: $.<field> has wrong type: got <got>, want <want>
```

That is the registry's own wrapper (`kernel/app/dispatch.go`:
`fmt.Errorf("operation %s input: %w", name, err)`) around the same schema
error, so the two surfaces say the same thing.

Three endpoints are behind it — the three the contract describes with a
request row:

| endpoint | the row |
|---|---|
| `run` | `run_request` |
| `mailbox_send` | `mail_draft` |
| `mailbox_ack` | `mail_ack` |

The door checks the **first** JSON value in the body — the value the handler's
decoder reads — so a body with trailing bytes after a valid object keeps
behaving exactly as it did.

### 2.3 What the door did not change

- The **handler** still reads its own struct, and the handler's own presence
  rules are untouched (`intent is required`, `text is required`, `"by" is
  required`).
- The **bytes on the wire** did not change, in either direction.
- The **empty body** is still `invalid JSON body: EOF`, and a malformed body is
  still the surface's own JSON error — the door does not take those over.
- The **surface's own refusals** (`request_too_large`) are still the surface's —
  with one alignment: the **ack** endpoint used to answer an oversized body
  with `400 invalid JSON body: request body too large` (it was the only one of
  the three that did not read the cap's error), and all three answer `413
  request_too_large` now. One oversized body is one answer, whichever endpoint
  it was sent to. Measured: the run and mailbox-send endpoints already answered
  `413 request_too_large` before this slice.

## 3. The measurement

| | before | after |
|---|---|---|
| endpoints on `/api/v1` that take a body | 4 | 4 |
| endpoints the contract describes with a request row | 3 | 3 |
| endpoints behind a contract-checked door | **0** | **3** |
| request fields the contract declares | 11 | 11 |
| request fields checked against the contract before the handler | **0** | **11** |
| mistyped fields refused in the registry's words | **0** | **11** |
| null fields refused | **0** | **11** |
| the refusal message | Go's decoder sentence | the registry's sentence |

The eleven fields are `run_request` (3), `mail_draft` (7) and `mail_ack` (1).

### 3.1 The known gap

`POST /api/v1/update/apply` takes a body the **contract does not describe**:
`{"version","sha256","url","notes"}`. There is no request row for it, so there
is no schema for the door to measure against — the endpoint is outside the
door's reach, and the door says so honestly rather than pretending to cover
it. Naming that body in the contract is the next step for it; it is not part
of this slice, because a new contract row reaches the generated SDK catalogs
and this slice is about the door.

## 4. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/restapi`: `TestTheRequestDoorRefusesAMistypedField` | every field of every request row is checked, and the refusal is the registry's sentence — for every endpoint the contract gives a request row |
| 2 | `kernel/restapi`: `TestTheRequestDoorRefusesANullField` | a null is refused, not silently the zero value |
| 3 | `kernel/restapi`: `TestTheRequestDoorSpeaksTheRegistrysLanguage` | the door's answer IS the registry's wrapper applied to the same schema error |
| 4 | `kernel/restapi`: `TestTheRequestDoorLetsAContractBodyThrough`, `TestTheRequestDoorAllowsAFieldTheContractDoesNotName`, `TestTheRequestDoorRefusesABodyThatIsNotAnObject`, `TestTheRequestDoorKeepsTheSurfacesOwnRefusals` | the door does not close the surface, and it does not take the surface's own errors over |
| 5 | `kernel/restapi`: `TestTheRequestRowsAgreeWithTheDaemonsRequestBodies` (W5.10) | the contract's rows and the daemon's structs still name the same fields |
| 6 | `kernel/contract/restapiapi`: `TestEveryRowRendersASchemaTheDoorCanCheck` | every field of every row renders a type the validator speaks, the schema carries no `required` list, and an undeclared field stays allowed |

Guard 1 is also the door's own ratchet: its endpoint list is derived from the
**contract**, so an endpoint the contract gives a request row but nobody wired
behind the door fails the test.

## 5. The mutations

Five mutations, five caught, all reverted (`.temp_files/w511_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the door is not wired into the run endpoint (it is handed the health endpoint's id) | *"the "run" endpoint: a mistyped "intent" said "invalid JSON body: json: cannot unmarshal number into Go struct field runRequest.intent of type string", want "operation run input: $.intent has wrong type: got number, want string""* — the door's absence is visible in the message |
| M2 | the contract renders a boolean as a `string` | *"the "mailbox_send" endpoint: a mistyped "help" said "invalid JSON body: json: cannot unmarshal string into Go struct field mailboxSendRequest.help of type bool", want "operation mailbox_send input: $.help has wrong type: got string, want boolean""* |
| M3 | the refusal drops the registry's `input:` | *"the door said "operation run $.intent has wrong type: got number, want string", the registry says "operation run input: $.intent has wrong type: got number, want string""* |
| M4 | the schema stops allowing a field the contract does not name | *"the "run" endpoint refused a body carrying an undeclared field: … "operation run input: $.extra_field is not allowed by schema""* |
| M5 | a wire type the mapping does not know renders no type at all | *"the "run_request" row's "stream" renders no type — the door would accept anything for it"* |

## 6. Preserved behaviour

- **The wire protocol did not change.** The SDKs send the same bytes; a body
  that fits the contract is decoded exactly as before.
- **One refusal moved**: the ack endpoint's oversized body answers `413
  request_too_large` now, like its two siblings, where it answered `400 invalid
  JSON body: request body too large` before (it was the only one of the three
  that did not read the cap's error).
- **The SDKs did not change.** Not a line of Python, TypeScript or Rust moved —
  the contract's *rows* did not move either, so the generated catalogs did not
  move (`tools/sdkgen -check` is in sync).
- **The console did not change.** This slice is the `/api/v1` surface; the
  console's `/api/*` surface is W5.8's and W5.9's.
- **The registry did not change.** Its door was already there; this slice gave
  the REST surface the same door, not a new one.

## 7. Verification

- `gofmt -l kernel cmd plugins tools sdk` — clean.
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — green.
- `go run ./tools/archcheck` — clean (340 packages, 0 violations).
- `go run ./tools/deadcodecheck` — clean (33 public SDK findings and 4
  cross-package test seams allowlisted).
- `go run ./tools/depscheck` — clean.
- `go run ./tools/sdkgen -check` — in sync.
- `go run ./tools/structure-md -check -out .project/STRUCTURE.generated` — in
  sync.
- `go run ./tools/openapi -check` — in sync.
- `go run ./tools/changelog-lint` — clean.
- `go run ./tools/docclaimscheck -base origin/main` — clean.
- `go run ./tools/docsindex` — clean.
- Python SDK: `python -m unittest discover -s tests` — **51** tests, green.
- TypeScript SDK: `npm test` (tsc + node:test) — **31** tests, green.
- Rust SDK: `cargo fmt --check` + `cargo test` — clean, **39** tests, green.

Measured: endpoints behind a contract-checked door **0 → 3**; request fields
checked against the contract before the handler **0 → 11**; mistyped fields
refused in the registry's words **0 → 11**; null fields refused **0 → 11**.
