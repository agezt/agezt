# W5.17 — the channel webhook family's vendor members get their contract and their door

## 1. What was wrong

W5.16 gave the **channel webhook family** its machinery: a contract
(`kernel/contract/channelhookapi`), a door (`kernel/httpserver`'s
`CheckJSONBody` / `RefuseJSONBody`), and a boot-time ratchet that holds a
member's contract to the struct the member actually reads. **One** of the
family's **fifteen** members was wired into it — the generic `webhook` plugin,
whose body is Agezt's own shape.

The other **six vendor members** of this slice were not:

| member | the body it reads | the body's shape | before this slice |
|---|---|---|---|
| `slack` | `slackEnvelope` | named | no contract row, no door |
| `discord` | `discordInteraction` | named | no contract row, no door |
| `whatsapp` | `waWebhook` | named | no contract row, no door |
| `line` | **anonymous** | anonymous | no contract row, no door |
| `imessage` | **anonymous** | anonymous | no contract row, no door |
| `zalo` | **anonymous** | anonymous | no contract row, no door |

So what happened to a mistyped body was still Go's `encoding/json` and the
plugin's own plain text:

- **`slack`, `discord`, `whatsapp`** answered a mistyped field with
  **400 — *"bad json"***: a sentence that names no field and no wanted type;
- **`line` and `imessage`** answered it with **200 and silence**: the body was
  decoded into the zero value, the message was dropped, and the caller was told
  the delivery had succeeded;
- **`zalo`** answered a body it could not read with **403** (its signature is
  computed over the timestamp *inside* the body, so an unreadable body has no
  timestamp to sign) — a security answer to a *shape* question;
- and a **null** was accepted everywhere: `encoding/json` maps a null to the
  field's zero value, so a null sender, a null text and a null timestamp all
  reached the agent as an empty string and a zero.

Two things were structurally missing as well:

1. **Three of the six bodies were anonymous.** A body read into an anonymous
   struct cannot be held to a contract *by name* — and the boot-time check
   compares the contract's schema with the **type** the plugin reads, so the
   plugin needs a named type to compare.
2. **Nothing held the contract to the member.** Even a contract row would have
   been a promise; W5.16's ratchet is what makes it a check.

## 2. What was built

### 2.1 Six more rows in the contract

`kernel/contract/channelhookapi` now names **seven** members: the generic one
and the six vendors. Each vendor's schema is the **vendor's** shape, field for
field — Slack's `event`, Discord's `data.options`, Meta's `entry[].changes[]`,
LINE's `events[]`, the bridge's `data`, Zalo's `sender` — and the contract may
not rename one.

The rules stay the registry's: **types, not presence** (no `required` list),
**`additionalProperties: true`** (a vendor's body carries fields Agezt never
looks at), and the registry's **type words** (an integer is a `number`, a
boolean is a `boolean`).

### 2.2 Two new type words, both earned

**"any" — a field the daemon reads as raw JSON.** Discord sends a *string* for
a text option and a *number* for a number one, and the plugin reads the value
only when the option's `type` says which. The daemon therefore cannot promise a
shape for it, and the door does not measure it. The contract says so
explicitly:

```json
"value": {"type": "any"}
```

and the shape guard holds the word to that: **"any" fits a raw field and
nothing else** — a schema that types a raw field is caught, and a schema that
calls a plain string *"any"* is caught too.

**A map's value shape.** Discord's resolved attachments are a map
(`map[string]discordAttachment`), and the plugin reads every value. The
contract says what a value looks like:

```json
"attachments": {"type": "object", "additionalProperties": {"type": "object", "additionalProperties": true, "properties": {"url": {"type": "string"}, "content_type": {"type": "string"}, "filename": {"type": "string"}}}}
```

and the guard now holds a map-shaped field to that shape: a schema that names
no shape for the values leaves every value unmeasured, and the guard says so.

### 2.3 Three anonymous bodies get a name

`lineWebhook`, `imessageWebhook` and `zaloWebhook` are now package-level named
types, with the fields and the JSON tags they had. A named type is what the
boot-time check compares against.

### 2.4 The six members are wired

Each member now does the same three things, in the same order:

1. **The signature is verified first** — a body is only read by someone who
   proved they may send it. A Zalo body is the exception that proves the rule:
   its signature covers the timestamp *inside* the body, so the timestamp is
   read from the raw bytes before the body is decoded.
2. **The door measures the body** against the contract's schema, before the
   member reads it and before any answer is written:

   ```go
   shape, _ := channelhookapi.Body(channelhookapi.MemberSlack)
   if err := httpserver.CheckJSONBody(body, shape, channelhookapi.MemberSlack, &env); err != nil {
       httpserver.RefuseJSONBody(w, err, "bad json")
       return
   }
   ```

3. **The member asks the contract, when it is built**, whether the two still
   agree on the body's shape:

   ```go
   if err := channelhookapi.CheckShape(channelhookapi.MemberSlack, reflect.TypeOf(slackEnvelope{})); err != nil {
       panic("channels/slack: the contract and the plugin disagree: " + err.Error())
   }
   ```

The refusal carries the registry's words, in the member's own plain text:

```
operation slack input: $.event.text has wrong type: got number, want string
operation whatsapp input: $.entry[0].changes[0].value.messages[0].from has wrong type: got number, want string
operation line input: $.events[0].message.text has wrong type: got number, want string
operation imessage input: $.data.text has wrong type: got number, want string
operation zalo input: $.sender.id has wrong type: got number, want string
operation discord input: $.id has wrong type: got number, want string
```

## 3. The measurement

Measured with the **same probe** on both trees (the probe walks every member's
schema, builds a body that fits it, and mistypes one field at a time — and
nulls it, one at a time — then measures it against the family's own door, the
validator the operation registry uses). The "before" side is the tree at
`origin/main` (`0f2f4c07`, the W5.16 merge).

| | before | after |
|---|---|---|
| members the contract names a body for | **1** | **7** |
| request fields checked before the plugin reads them | **6** | **62** |
| probes refused with the registry's sentence (62 wrong + 62 null) | **12** | **124** |
| nulls refused instead of silently zero | **6** | **62** |
| probes accepted | **0** | **0** |
| the family's members with a contract row | 1 of 15 | **7 of 15** |

The plugin side, counted in the sources on the same two trees:

| | before | after |
|---|---|---|
| members whose inbound handler measures the body against the contract | **1** | **7** |
| members that hold the contract to their own type at boot | **1** | **7** |

The **62** measured fields are the six vendors' **56** fields plus the generic
member's **6**; Discord's raw option value is the one field the door does not
measure, and the contract says so with the word *"any"*.

### 3.1 What did not change

- **The wire protocol.** A body that fits the contract reaches the member
  exactly as before, and each member's reply is untouched (Slack's challenge
  handshake, Discord's PONG, WhatsApp's 200, LINE's 200, the bridge's 200,
  Zalo's 200).
- **The signature is still checked FIRST**, with each member's own scheme:
  Slack's `v0=` HMAC over the timestamp, Discord's Ed25519, Meta's
  `sha256=` HMAC, LINE's base64 HMAC, the bridge's shared secret, Zalo's
  `mac=` HMAC. An unsigned body is still refused by the member.
- **The allowlist and the replay guards** are untouched: the dedup rings, the
  freshness windows, the allowlist's 403.
- **The vendor's names are the vendor's.** No field was renamed.
- **The strictness.** The whole body must still be one JSON value, the way it
  always was.

### 3.2 The refusals that did change

| the body | before | after |
|---|---|---|
| a mistyped field (slack, discord, whatsapp) | 400 — *"bad json"* | 400 — *"operation slack input: $.event.text has wrong type: got number, want string"* |
| a mistyped field (line, imessage) | **200** — swallowed | 400 — *"operation line input: $.events[0].message.text has wrong type: got number, want string"* |
| a mistyped field (zalo) | 403 — the signature could not be computed | 400 — *"operation zalo input: $.sender.id has wrong type: got number, want string"* |
| a null field | **200** — the zero value | 400 — *"… has wrong type: got null, want string"* |
| a body that is not JSON | 400 — *"bad json"* (line, imessage: 200) | 400 — *"bad json"* |

The status code for a shape refusal did not change: a body refusal is still a
**400**.

### 3.3 One ordering decision, stated

**Zalo's freshness check moved behind the door.** Zalo's signature covers the
timestamp *inside* the body, so the signature must be verified before the body
is decoded — but the freshness window (a replay guard) is now checked **after**
the door, so that a body with a *wrong shape* is answered with the registry's
sentence instead of with a replay refusal. The security order is unchanged:
the signature is still verified before the body is measured, and a stale body
is still refused with 403.

## 4. The guards

**Thirty-seven new guards** — two in the contract, thirty-five in the six
members — on top of W5.16's nine.

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/channelhookapi`: `TestTheShapeGuardHoldsARawField` | *"any"* is the contract's word for a raw field: a schema that types a raw field is caught, and a schema that calls a plain string *"any"* is caught too |
| 2 | `kernel/contract/channelhookapi`: `TestTheShapeGuardHoldsAMapShape` | a map-shaped field's values are measured: a schema that names no shape for them is caught, and a schema that mistypes one is caught |
| 3 | each member: `TestTheContractAndTheDaemonNameTheSameBody` | the contract's schema and the member's own struct name the **same fields with the same types**, in both directions — the same check the member makes when it is built |
| 4 | each member: `TestTheDoorRefusesAMistypedField` | a mistyped field is refused with the registry's words, naming the field and the wanted type |
| 5 | each member: `TestTheDoorRefusesANullField` | a null is refused too, instead of becoming the field's zero value |
| 6 | each member: `TestTheDoorLetsAContractBodyThrough` | the door does not close the member: a body that fits still gets the member's own answer |
| 7 | each member: `TestTheDoorAllowsAFieldTheContractDoesNotName` | an undeclared field stays allowed — a body is the stable subset of what the daemon reads |
| 8 | line, imessage, zalo: `TestTheDoorRefusesABodyThatIsNotJSON` | a body that is not JSON is refused in the member's own words |
| 9 | `slack`, `discord` — `TestTheMemberKeepsItsOwnRefusals` | the member's own refusals are still the member's |

W5.16's four contract guards still hold, and they now hold **seven** members
instead of one: the member table is well-formed, every body schema is a schema
the door can check (**at every level**, and now including a map's value
shape), the lookup answers the table, and the shape guard catches a schema that
is wrong on purpose.

## 5. The mutations

Eight mutations, eight caught, all reverted
(`.temp_files/w517_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | **the door core stops measuring the body** (`kernel/httpserver/body.go` no longer validates) | *"a mistyped field said "bad json", want it to contain "operation slack input: $.event.text has wrong type: got number, want string""* |
| M2 | the contract mistypes a vendor's field (Slack's `event.text` becomes a number) | *"the contract and the plugin disagree: "event.text" is declared as [number], the daemon reads it as [string]"* |
| M3 | the contract drops a field the daemon reads (Slack's `files[].mimetype`) | *"the contract and the plugin disagree: "event.files[].mimetype" is read by the daemon but the schema does not name it"* |
| M4 | the contract names a field the daemon does not read (Zalo's `nope`) | *"the contract and the plugin disagree: "nope" is named by the schema but the daemon does not read it"* |
| M5 | a vendor's schema stops allowing an undeclared field (WhatsApp) | *"the member refused a body carrying an undeclared field: operation whatsapp input: $.extra_field is not allowed by schema"* |
| M6 | a vendor's schema asks for presence (`required`) | *"the "slack" member's schema carries the required list [type] — the door checks types, not presence"* |
| M7 | the map's value shape disappears (Discord's attachments) | *"the contract and the plugin disagree: "data.resolved.attachments" is a map in the daemon but the schema names no shape for its values"* |
| M8 | a raw field is given a type (Discord's option `value`) | *"the contract and the plugin disagree: "data.options[].value" is declared as [string], the daemon reads it as [any]"* |

M1 mutates the **shared core** rather than one member, so it proves the door
is load-bearing for every member at once. M2, M3, M4, M7 and M8 are caught by
the check the member makes **when it is built**: the contract and the plugin
disagree, and the daemon stops. That is the point of moving the ratchet into
the production path — a schema that mistyped a field cannot agree with itself
and pass.

## 6. Preserved behaviour

- **The vendor's wire shapes did not change.** The schemas name the fields the
  vendors send, and the members decode into the same structs as before.
- **The security order did not change.** The signature is verified before the
  body is measured; the allowlist, the dedup rings and the freshness windows
  are untouched.
- **The SDKs did not change.** The channel webhook family is not part of the
  SDK contract: the SDKs speak `/api/v1`, and the callers of this family are
  the vendors' own webhook deliveries.

## 7. What is left

Six vendor members of the family are still outside the door, and the next slice
adds them to the same table, one row at a time:

| member | the body it reads | notes |
|---|---|---|
| `chatwebhook` | anonymous | two dialects: Google Chat's JSON and Mattermost's form |
| `dingtalk` | anonymous | the signature is header-only |
| `feishu` | anonymous ×2 | the URL-verification handshake and the event |
| `nextcloudtalk` | anonymous | the signature covers a random prefix and the body |
| `onebot` | anonymous | the ids are `json.Number` — the contract needs a number word for them |
| `whatsappgw` | anonymous ×2 | two backends (WAHA and Evolution), two different bodies |

**`wecom`** (an XML callback) and **`sms`** (a form) take no JSON body and are
named in the contract as members that take none. The **ACP surface**
(`kernel/acp`) is a stdio JSON-RPC server, not an HTTP one, and stays outside
the programme until that is decided on purpose.

## 8. Verification

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

Measured: members the contract names a body for **1 → 7**; request fields
checked before the plugin reads them **6 → 62**; probes refused with the
registry's sentence **12 → 124**; nulls refused **6 → 62**; members behind a
door **1 → 7**; members whose contract and daemon are held together at boot
**1 → 7**; the family's members with a contract row **1 of 15 → 7 of 15**.
