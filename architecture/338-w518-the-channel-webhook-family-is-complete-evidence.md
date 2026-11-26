# W5.18 — the channel webhook family is complete: every member has a contract and a door

## 1. What was wrong

W5.16 gave the **channel webhook family** its machinery — a contract
(`kernel/contract/channelhookapi`), a door (`kernel/httpserver`'s
`CheckJSONBody` / `RefuseJSONBody`) and a boot-time ratchet that holds a
member's contract to the struct the member reads. W5.17 wired the family's
**six** vendor members into it. This slice wires the **last six** — and
completes the roster.

| member | the body it reads | before this slice |
|---|---|---|
| `chatwebhook` | anonymous (Google Chat) | no contract row, no door |
| `dingtalk` | anonymous | no contract row, no door |
| `feishu` | anonymous ×2 (handshake + event) | no contract row, no door |
| `nextcloudtalk` | anonymous | no contract row, no door |
| `onebot` | anonymous, ids as `json.Number` | no contract row, no door |
| `whatsappgw` | anonymous ×2 (WAHA + Evolution) | no contract row, no door |

So what happened to a mistyped body was still Go's `encoding/json` and the
plugin's own plain text: a **200 and silence** in `dingtalk`, `nextcloudtalk`,
`onebot`, `whatsappgw` and `chatwebhook` (the body was swallowed and the
vendor was told the delivery had succeeded), and a **403** in `feishu`, whose
token is inside the body — an unreadable body has no token to verify. A
**null** was accepted everywhere: it became the field's zero value.

Two of the six had a shape the contract could not name as it stood, and both
needed a decision rather than a transcription:

1. **One route, two bodies** (`feishu`'s handshake and its event,
   `whatsappgw`'s two gateway backends). The door measures **one** schema, so
   the plugin has to know which member it is *before* the body is measured —
   and a plugin that peeks at the body to find out has already read it. The
   answer: **one body type that holds both shapes**, and therefore one member.
   The plugin reads both shapes in the same struct anyway, so the two-way
   shape check still holds — and a vendor's dialect is a field of the body,
   not a second contract.
2. **A numeric id** (`onebot`'s `message_id`, `user_id`, `group_id` are read
   as `json.Number`). The shape guard read a `json.Number` as a **string** —
   its kind is string — so a contract that named the ids as numbers would not
   have agreed with the daemon, and a contract that named them as strings
   would have **refused the numbers the vendor actually sends**. The guard now
   knows that a `json.Number` is read as a **number**.

## 2. What was built

### 2.1 Six more rows in the contract

The contract now names **thirteen** members with a JSON body — the generic one
and the twelve vendors — in the **vendor's own field names**, and the door
measures them with the same validator the registry uses:

```
operation chatwebhook input: $.message.text has wrong type: got number, want string
operation dingtalk input: $.text.content has wrong type: got number, want string
operation feishu input: $.event.message.message_id has wrong type: got number, want string
operation nextcloudtalk input: $.object.content has wrong type: got number, want string
operation onebot input: $.user_id has wrong type: got string, want number
operation whatsappgw input: $.payload.body has wrong type: got number, want string
```

### 2.2 The guard learns a number

```go
// numberType is the Go type a numeric id is read as. A json.Number is a
// string underneath, but it only ever HOLDS a number — the vendor sends it as
// one, and the decoder refuses a JSON string for it — so the registry's word
// for it is "number", not "string".
```

The guard holds the word in both directions: a contract that calls a
`json.Number` a string is caught, and a contract that calls a plain string a
number is caught.

### 2.3 The roster is one table: 15 of 15

The family has **fifteen** members. Thirteen take a JSON body and are behind
the door. Two do not:

- **`wecom`** reads an **XML** callback (`xml.Unmarshal`),
- **`sms`** reads a **form** (`ParseForm` over the captured bytes).

They are now **named in the contract** as members that take no JSON body, so
the family's roster is one table and the door only ever has to measure JSON —
and each of them says so **when it is built**:

```go
// This member reads an XML callback, not a JSON body: the contract must
// not name one for it. A contract that grew a JSON body for WeCom would
// stop the daemon HERE, instead of quietly measuring a body this member
// never reads.
if _, ok := channelhookapi.Body(channelhookapi.MemberWeCom); ok {
    panic("channels/wecom: the contract names a JSON body for a member that reads XML")
}
```

So **every** member of the family now holds the contract to itself: the
thirteen JSON members check the **shape** they read, and the two others check
that the contract names them **no body**.

### 2.4 The six members are wired

Each member does the same three things, in the same order: the **signature is
verified first**, the **door measures the body** before the member reads it and
before any answer is written, and the member asks the **contract, at boot**,
whether the two still agree on the body's shape.

Two of them needed the order spelled out, and both are the honest consequence
of where their credential lives:

- **`feishu`**: the token is **inside** the body, so it is read from the raw
  bytes first — a tiny probe, exactly as Zalo reads its timestamp — and
  verified **before** the door. A body that is not JSON at all carries no
  token, so there is nothing to verify and the **door** is what refuses it.
- **`chatwebhook`**: the member has two dialects, and only one of them is a
  JSON body. Google Chat's is measured; Mattermost's **form** is not — the
  door only ever measures JSON.

## 3. The measurement

Measured with the **same probe** on both trees (the probe walks every member's
schema, builds a body that fits it, and mistypes one field at a time — and
nulls it, one at a time — then measures it against the family's own door). The
"before" side is the tree at `origin/main` (`e484ed09`, the W5.17 merge).

| | before | after |
|---|---|---|
| members the contract names a body for | **7** | **13** |
| request fields checked before the plugin reads them | **62** | **108** |
| probes refused with the registry's sentence (108 wrong + 108 null) | **124** | **216** |
| nulls refused instead of silently zero | **62** | **108** |
| probes accepted | **0** | **0** |
| members behind a door | **7** | **13** |
| members holding the contract to themselves at boot | **7** | **15** |
| the family's members in the contract | 7 of 15 | **15 of 15** |

The **46** new measured fields are the six members' **46** fields; the
vendors' share of the family's measured fields is now **102** (and the generic
member's **6**).

### 3.1 What did not change

- **The wire protocol.** A body that fits the contract reaches the member
  exactly as before, and each member's reply is untouched — DingTalk's 200,
  Feishu's challenge echo and its 200, Nextcloud Talk's 200, OneBot's 200, the
  gateway's 200, Google Chat's 200.
- **The signature is still checked FIRST**, with each member's own scheme:
  DingTalk's header-only `timestamp`+`sign` HMAC, Feishu's in-body token,
  Nextcloud Talk's HMAC over a random prefix and the body, OneBot's
  `sha1=` HMAC, the gateway's `X-Webhook-Secret`, Google Chat's `?token=`.
- **The allowlist and the replay guards** are untouched: the dedup rings, the
  freshness windows, the allowlist's refusals.
- **The vendor's names are the vendor's.** No field was renamed; the two
  dialects of a member are fields of one body, not two contracts.

### 3.2 The refusals that did change

| the body | before | after |
|---|---|---|
| a mistyped field | 200 — swallowed (feishu: 403) | 400 — *"operation onebot input: $.user_id has wrong type: got string, want number"* |
| a null field | 200 — the zero value | 400 — *"… has wrong type: got null, want …"* |
| a body that is not JSON | 200 — swallowed | 400 — *"bad json"* |

The status code for a shape refusal did not change: a body refusal is still a
**400**.

One case is worth naming because it was already broken and is now loud:
**OneBot's ids are read as `json.Number`**, and the decoder has always
refused a JSON *string* for them — silently, as a 200 with no message. Such a
body is now refused with the registry's words, which is what the vendor needs
to see.

## 4. The guards

**Forty new guards** — two in the contract, thirty-eight in the six members —
on top of W5.16's nine and W5.17's thirty-seven. The family now carries
**eighty** door guards.

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/channelhookapi`: `TestTheShapeGuardReadsANumberAsANumber` | a `json.Number` is read as a **number**: a contract that calls it a string is caught |
| 2 | `kernel/contract/channelhookapi`: `TestTheMembersThatTakeNoJSONBody` | `wecom` and `sms` are in the roster and name **no** body, and the lookup answers none for them |
| 3 | each member: `TestTheContractAndTheDaemonNameTheSameBody` | the contract's schema and the member's own struct name the **same fields with the same types**, in both directions |
| 4 | each member: `TestTheDoorRefusesAMistypedField` | a mistyped field is refused with the registry's words, naming the field and the wanted type |
| 5 | each member: `TestTheDoorRefusesANullField` | a null is refused too, instead of becoming the field's zero value |
| 6 | each member: `TestTheDoorLetsAContractBodyThrough` | the door does not close the member: a body that fits still gets the member's own answer |
| 7 | each member: `TestTheDoorAllowsAFieldTheContractDoesNotName` | an undeclared field stays allowed — a body is the stable subset of what the daemon reads |
| 8 | each member: `TestTheDoorRefusesABodyThatIsNotJSON` | a body that is not JSON is refused in the member's own words |
| 9 | feishu, nextcloudtalk: the credential is verified **before** the door | an unsigned or untokened body is still 403 / 401 — the door never runs first |

## 5. The mutations

Eight mutations, eight caught, all reverted
(`.temp_files/w518_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | **the door core stops measuring the body** | *"a mistyped field said "bad json", want "operation dingtalk input: $.text.content has wrong type: got number, want string""* |
| M2 | the contract calls a numeric id a string | *"the contract and the plugin disagree: "user_id" is declared as [string], the daemon reads it as [number]"* |
| M3 | the contract drops a field the daemon reads (Feishu's `chat_id`) | *"the contract and the plugin disagree: "event.message.chat_id" is read by the daemon but the schema does not name it"* |
| M4 | the contract names a field the daemon does not read | *"the contract and the plugin disagree: "nope" is named by the schema but the daemon does not read it"* |
| M5 | a vendor's schema stops allowing an undeclared field | *"the member refused a body carrying an undeclared field: operation dingtalk input: $.extra_field is not allowed by schema"* |
| M6 | a vendor's schema asks for presence | *"the "chatwebhook" member's schema carries the required list [type] — the door checks types, not presence"* |
| M7 | the shape guard stops reading a `json.Number` as a number | *"the contract and the plugin disagree: "message_id" is declared as [number], the daemon reads it as [string]"* |
| M8 | the contract drops one dialect's field | *"the contract and the plugin disagree: "data.key.remoteJid" is read by the daemon but the schema does not name it"* |

M1 mutates the **shared core** rather than one member, so it proves the door
is load-bearing for every member at once. M2, M3, M4, M7 and M8 are caught by
the check the member makes **when it is built**: the contract and the plugin
disagree, and the daemon stops.

## 6. Preserved behaviour

- **The vendor's wire shapes did not change.** The schemas name the fields the
  vendors send, and the members decode into the same structs as before.
- **The security order did not change.** The signature is verified before the
  body is measured — including the two members whose credential lives inside
  the body.
- **The SDKs did not change.** The channel webhook family is not part of the
  SDK contract: the SDKs speak `/api/v1`, and the callers of this family are
  the vendors' own webhook deliveries.

## 7. What is left

**The channel webhook family is done: fifteen of fifteen members are in the
contract, and every one of them holds it to itself.** What is left is the
programme's open decision, named here so it is deliberate rather than
forgotten: the **ACP surface** (`kernel/acp`) is a **stdio** JSON-RPC server,
not an HTTP one, and it answers a mistyped `params` with a JSON-RPC error
(*"invalid params: …"*). The programme's sentence is about HTTP surfaces, so
it needs an explicit decision — either it stays out, or the door's core is
made transport-agnostic enough for a stdio line to be measured against a
schema too.

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

Measured: members the contract names a body for **7 → 13**; request fields
checked before the plugin reads them **62 → 108**; probes refused with the
registry's sentence **124 → 216**; nulls refused **62 → 108**; members behind
a door **7 → 13**; members holding the contract to themselves at boot **7 → 15**;
the family's members in the contract **7 of 15 → 15 of 15**.
