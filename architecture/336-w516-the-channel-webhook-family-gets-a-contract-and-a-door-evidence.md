# W5.16 — the channel webhook family gets a contract and a door

## 1. What was wrong

The **channel webhook family** is the set of plugins under `plugins/channels`
that serve an inbound webhook: **15** of them, each opening its **own**
`http.ServeMux` for one route. There is no shared surface, no shared contract
and no shared door — each plugin reads the body into whatever struct it
happened to use, and **nine of the thirteen JSON bodies** are read into an
**anonymous struct**.

What happened to a mistyped body was therefore the plugin's own plain text, and
never the registry's words. Measured on the tree at `origin/main` (`ff5ee100`),
on the family's generic member (the `webhook` plugin — the one whose body is
Agezt's own shape, and the shape every other member's plugin follows):

| the body | the old answer |
|---|---|
| `POST /webhook {"text":42,…}` | 400 — *"bad json"* |
| `POST /webhook {"sender":null,…}` | **200** — the null became an empty sender and the message was delivered |
| `POST /webhook {"id":null,…}` | **200** — the null became an empty id |
| `POST /webhook {"ts_ms":null,…}` | **200** — the null became a zero timestamp |
| `POST /webhook {"images":null,…}` | **200** — the null became no attachments |
| `POST /webhook {"channel_id":null,…}` | 400 — *"channel_id and text are required"* (the handler's own rule) |
| `POST /webhook {"text":null,…}` | 400 — *"channel_id and text are required"* |
| `POST /webhook []` | 400 — *"bad json"* |

Three things were wrong with that:

1. **The words were the plugin's, not the registry's.** A caller of a channel
   webhook learned a different language from a caller of `/api/v1`, of the
   agent gateway or of the OpenAI-compatible surface. Nothing in the answer
   said *which* field was wrong, or what it wanted.
2. **A null was mostly accepted.** Four of the six measured nulls were
   **delivered** with a 200 — a null sender, a null id, a null timestamp, null
   attachments — while the other two were refused by the handler's own
   presence rule, in the handler's own words.
3. **The family had no contract at all.** Nothing named its bodies, so nothing
   could check them: the thirteen JSON bodies name **126** fields between them
   and **11** of the **15** body structs are anonymous.

## 2. What was built

### 2.1 The family gets a contract

`kernel/contract/channelhookapi` holds the family's inbound vocabulary as
data: a **member** (an id and the JSON Schema of its inbound body), and the
lookup the door uses.

```go
type Member struct {
	ID   string
	Body json.RawMessage
}
```

The rules are the registry's: **types, not presence** (no `required` list — a
missing field stays the plugin's business: *"channel_id and text are
required"*); **`additionalProperties: true`** (a body is the stable subset of
what the daemon reads, and a vendor's body carries fields Agezt never looks
at); and the **registry's type words** (an integer is a `number`).

A member's body is the **vendor's** shape, so the contract names the fields
the vendor names and may not rename one. Only the generic member's body is
Agezt's own.

### 2.2 The door is the transport's shared core, and it is strict

The family's door is `kernel/httpserver`'s — the same core the REST surface,
the console, the agent gateway and the OpenAI-compatible surface use — in two
new functions:

```go
func CheckJSONBody(body []byte, shape json.RawMessage, op string, target any) error
func RefuseJSONBody(w http.ResponseWriter, err error, notJSON string)
```

`CheckJSONBody` is the sibling of `DecodeJSONBody` for a surface that must see
the **raw bytes** before the body is parsed: a channel webhook verifies a
signature over the bytes first, and only then may the body be read. Two things
make it different from its sibling, and both are the surface's own behaviour
rather than the door's:

- the whole body must be **one** JSON value — a body with trailing bytes is
  refused here, the way it always was on a surface that decoded it strictly;
- the body is **measured before it is decoded**, so a mistyped field is refused
  with the registry's words and never with the decoder's.

`RefuseJSONBody` writes the refusal in the caller's voice: the registry's
sentence as plain text when the shape is wrong, and the caller's own words
(*"bad json"*) when the body is not JSON at all.

### 2.3 The contract and the member are held together — at boot

The family's members are many packages, so the guard that holds a member's
contract to its own struct is written **once**, in the contract
(`CheckShape`), and the member asks the contract when it is **built**:

```go
if err := channelhookapi.CheckShape(channelhookapi.MemberWebhook, reflect.TypeOf(inboundEnvelope{})); err != nil {
    panic("channels/webhook: the contract and the plugin disagree: " + err.Error())
}
```

A contract that mistyped a field, dropped a field the daemon reads, or named a
field the daemon does not read **stops the daemon** instead of quietly
disagreeing with the member that reads it. The comparison holds the **types**
as well as the names, and it walks the nested shapes too.

### 2.4 The generic member is wired

`plugins/channels/webhook` now measures its body against the contract before it
reads it, after the signature is verified:

```go
shape, _ := channelhookapi.Body(channelhookapi.MemberWebhook)
if err := httpserver.CheckJSONBody(body, shape, channelhookapi.MemberWebhook, &env); err != nil {
	httpserver.RefuseJSONBody(w, err, "bad json")
	return
}
```

The refusal carries the registry's words, in the member's own plain text:

```
operation webhook input: $.text has wrong type: got number, want string
```

## 3. The measurement

Measured on the tree at `origin/main` (`ff5ee100`) with the same probe that
measured the "after" side: the generic member's **6** fields, each probed
twice (a wrong type and a null), plus the root.

| | before | after |
|---|---|---|
| members the contract names a body for | **0** | **1** |
| members behind a door | **0** | **1** |
| request fields checked before the plugin reads them | **0** | **6** |
| probes refused with the registry's sentence (6 wrong + 6 null) | **0** | **12** |
| nulls refused instead of silently zero | **2** | **6** |
| nulls **accepted** with a 200 | **4** | **0** |
| members whose contract and daemon are held together at boot | **0** | **1** |
| the family's members with a contract row | 0 of 15 | **1 of 15** |

The family-wide numbers were measured on the same tree: **15** members serve
an inbound webhook, **13** of them take a **JSON** body (`wecom` takes XML,
`sms` a form), and those **13** bodies are read into **15** structs — **11
anonymous** and **4 named** — naming **126** JSON fields between them.

**One member of the fifteen is wired in this slice**: the generic one, whose
body is Agezt's own shape and whose plugin the other twelve follow. The
vendor's members keep their own shapes and their own names — and the next
slice adds them to the same contract, one table row at a time.

### 3.1 What did not change

- **The valid body.** It still answers 200 with the same reply object.
- **The signature is still checked FIRST** — a body is only read by someone who
  proved they may send it. An unsigned body is still 401 *"bad signature"*.
- **The allowlist.** A channel that is not allowlisted is still 403.
- **The member's own presence rule.** A body without a channel is still 400
  *"channel_id and text are required"* — the door does not take the handler's
  business away.
- **The replay guards.** A stale `ts_ms` is still 401 *"stale timestamp"*, and
  a repeated `id` is still the member's *"duplicate"* answer.
- **A body that is not JSON** is still 400 *"bad json"* — the decoder's error
  is the member's own business, not the door's.
- **The strictness.** A body with trailing bytes after a valid object is still
  refused, the way it always was.

### 3.2 The refusals that did change

| the body | before | after |
|---|---|---|
| a mistyped field | 400 — *"bad json"* | 400 — *"operation webhook input: $.text has wrong type: got number, want string"* |
| a null field | 200 (4 of 6), or 400 in the member's own words | 400 — *"operation webhook input: $.text has wrong type: got null, want string"* |
| a body that is not an object | 400 — *"bad json"* | 400 — *"operation webhook input: $ has wrong type: got array, want object"* |

The status code did not change: a body refusal is still a **400**.

## 4. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/channelhookapi`: `TestTheMemberTableIsWellFormed` | every member has an id and a body, and no two claim the same id |
| 2 | `kernel/contract/channelhookapi`: `TestEveryBodySchemaIsASchemaTheDoorCanCheck` | every body schema parses, is an object, carries no `required` list, allows an undeclared field, and names every property **at every level** with a type |
| 3 | `kernel/contract/channelhookapi`: `TestTheLookupAnswersTheTable` | the lookup the door uses answers the schema the table states — and answers none for a member the contract does not know |
| 4 | `kernel/contract/channelhookapi`: `TestTheShapeGuardCatchesAWrongSchema` | the shape guard **catches** a schema that mistypes a field, drops one, or names one the daemon does not read — the guard cannot agree with a wrong schema |
| 5 | `plugins/channels/webhook`: `TestTheContractAndTheDaemonNameTheSameBody` | the contract's schema and the plugin's own struct name the **same fields with the same types**, in both directions — the same check the member makes when it is built |
| 6 | `plugins/channels/webhook`: `TestEveryFieldIsGuarded` | every field the contract names has a value that does not fit it, so no field of the body is left unmeasured |
| 7 | `plugins/channels/webhook`: `TestTheDoorRefusesAMistypedField`, `TestTheDoorRefusesANullField` | a mistyped field — and a null — is refused with the registry's words, for every field of the body |
| 8 | `plugins/channels/webhook`: `TestTheDoorLetsAContractBodyThrough`, `TestTheDoorAllowsAFieldTheContractDoesNotName`, `TestTheDoorRefusesABodyThatIsNotAnObject` | the door does not close the member, an undeclared field stays allowed, and a body that is not an object is refused |
| 9 | `plugins/channels/webhook`: `TestTheMemberKeepsItsOwnRefusals` | an unsigned body is still a bad signature, a body without a channel is still the member's own refusal, and a channel that is not allowlisted is still a 403 |

Guard 4 is the one that keeps the rest honest: a guard that cannot fail is not
a guard, so the contract proves its own shape guard on a schema that is wrong
on purpose.

Guard 6 is the family's coverage rule: the guards walk the fields the
**contract** names, so a field the contract adds without a value to mistype is
a field nobody measures — and the coverage test says so.

## 5. The mutations

Six mutations, six caught, all reverted (`.temp_files/w516_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the door stops measuring the body (an empty schema) | *"a mistyped "text" said "bad json", want it to contain "operation webhook input: $.text has wrong type: got ""* |
| M2 | the contract mistypes a field (`text` becomes a number) | *"the contract and the plugin disagree: "text" is declared as [number], the daemon reads it as [string]"* |
| M3 | the contract drops a field the daemon reads (`sender`) | *"the contract and the plugin disagree: "sender" is read by the daemon but the schema does not name it"* |
| M4 | the contract names a field the daemon does not read (`extra`) | *"the contract and the plugin disagree: "extra" is named by the schema but the daemon does not read it"* |
| M5 | the contract's schema stops allowing an undeclared field | *"the member refused a body carrying an undeclared field: operation webhook input: $.extra_field is not allowed by schema"* |
| M6 | the contract's schema asks for presence (`required`) | *"the "webhook" member's schema carries the required list [text] — the door checks types, not presence"* |

M2, M3 and M4 are caught by the check the member makes **when it is built**:
the contract and the plugin disagree, and the daemon stops. That is the point
of moving the ratchet into the production path — a schema that mistyped a
field cannot agree with itself and pass.

## 6. Preserved behaviour

- **The wire protocol did not change.** A body that fits the contract reaches
  the member exactly as before, and the reply object is untouched.
- **The security order did not change.** The signature is verified before the
  body is read, the allowlist still gates who may drive the agent, and the
  replay guards still answer.
- **The vendor's names are the vendor's.** A member's body is the vendor's
  shape; the contract names the fields the vendor names and may not rename one.
- **The SDKs did not change.** The channel webhook family is not part of the
  SDK contract: the SDKs speak `/api/v1`, and the callers of this family are
  the vendors' own webhook deliveries.

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
- The Rust SDK (`cargo fmt --check` + `cargo test`) was **not** run on this host
  — `cargo` is not installed here — and the CI job `rust-sdk` runs it. Nothing
  in this slice touches the Rust SDK: the family is not part of the SDK
  contract, and the SDK catalogs did not move (`sdkgen -check` is in sync).

Measured: members the contract names a body for **0 → 1**; members behind a
door **0 → 1**; request fields checked before the plugin reads them **0 → 6**;
probes refused with the registry's sentence **0 → 12**; nulls refused **2 → 6**;
nulls accepted **4 → 0**; members whose contract and daemon are held together
at boot **0 → 1**.
