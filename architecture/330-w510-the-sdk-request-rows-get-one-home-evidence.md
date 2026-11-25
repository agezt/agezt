# W5.10 — the SDK's request rows get one home

## 1. What was wrong

W5.8 gave the **browser's** request bodies their types and W5.9 gave the
**daemon's** request side the same truth. The half that was left was the
**SDK's own**: the clients that speak the daemon's native REST surface
(`/api/v1`) — Python, TypeScript and Rust.

The REST contract (`kernel/contract/restapiapi`) already named **3** request
rows — `run_request`, `mail_draft`, `mail_ack` — and the generated catalogs
already carried them. But the **clients did not use them**. Measured over the
tree at `origin/main`:

| SDK | request bodies it spells by hand | request bodies it builds from the row |
|---|---|---|
| Python (`sdk/python/agezt/client.py`) | **4** | 0 |
| TypeScript (`sdk/typescript/src/client.ts`) | **4** | 0 |
| Rust (`sdk/rust/src/client.rs`) | 0 | 3 |
| Go (`sdk/`) | 0 | — (it speaks the control plane) |

So **8** request bodies were written out by hand, in two languages, naming
**24** fields between them — the same hole the browser had before W5.8, in the
clients this slice's types are meant to reach. A TypeScript client wrote

```ts
const body: Record<string, unknown> = { text: draft.text };
if (draft.from) body.from = draft.from;
if (draft.replyTo) body.reply_to = draft.replyTo;
```

and a Python client wrote the same body as a `Dict[str, Any]`. Nothing typed
the wire: a field renamed in the contract was renamed **again**, by hand, in
every client — and the row the contract declared sat unused beside it.

The handoff note was wrong about one SDK, and the measurement corrected it:
**Rust already held the line.** Its client has always built its bodies from
the generated rows (`RunRequest`, `MailDraft`, `MailAck` and their `to_value`),
so the Rust catalog was the pattern the other two now follow — not a hole.

## 2. What was built

### 2.1 The contract renders the body; the clients send it

`restapiapi.RowBody` renders a request row into the bytes on the wire, and
`tools/sdkgen` renders the same rule into every SDK catalog:

- **TypeScript** — a `toWire` function per request row
  (`runRequestToWire`, `mailDraftToWire`, `mailAckToWire`);
- **Python** — a `to_wire` method on each request row (the request rows are
  dataclasses now, so they can carry one);
- **Rust** — unchanged: its catalog has always had `to_value`.

The rule is one rule, in all three:

> a **required** field is always written; an **optional** field only when it
> holds something (an empty string, a zero, a false, an empty list).

Because an absent optional field and an empty one are not the same request,
and because the daemon reads an absent one as the zero value.

The TypeScript catalog also carries the **local-name map** of each request row
(`MAIL_DRAFT_NAMES` and its siblings: wire name → the name the language
calls it). The difference between the two spellings — the draft's `reply_to`
is `replyTo` in TypeScript, `from` is `from_` in Python — lives in the
catalog, so a client never spells a wire name.

The Python and TypeScript clients now build their bodies from the generated
rows and send the catalog's bytes. **The bytes on the wire did not change**:
the sparse rule reproduces the old omission exactly, so a client that sent
`{"intent": "x"}` still sends `{"intent": "x"}`.

### 2.2 One daemon bug the binding found

The daemon's `/api/v1` request bodies are decoded into its **own** structs —
a second home for the same vocabulary. Comparing them with the contract found
a field the daemon read and the contract could not see:

```go
type mailboxSendRequest struct {
	...
	CorrelationID string `json:"correlation_id"`
}
```

`correlation_id` is the correlation id the message's board event carries (the
`X-Agezt-Correlation` header is the fallback). The daemon has always read it;
the contract never named it, so **no SDK could send it** and no type could
describe it. The contract's `mail_draft` row names it now, and the SDKs can
send it, typed.

### 2.3 The golden fixture: the contract's bytes, read by three languages

`kernel/restapi`'s golden recorder already recorded what the daemon
**answers** into `sdk/fixtures/`. It now also records what it **accepts**:
`sdk/fixtures/requests.json`, rendered by the contract itself
(`restapiapi.RowBody`), so the fixture's shape is the contract's shape and
not a payload a test wrote itself.

Every SDK reads that fixture and checks that its own row serializes to
exactly those bytes:

| SDK | the check |
|---|---|
| Python | `MailDraft.from_dict(body).to_wire() == body` |
| TypeScript | `mailDraftToWire(row) == body` |
| Rust | `MailDraft::from_value(&body).to_value() == body` |

Three languages, one contract, the same bytes — the request side's version of
the golden fixture the SDKs already used for the responses.

## 3. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `tools/sdkgen`: `TestTheSDKsBuildTheirRequestBodiesFromTheCatalog` | an SDK client may not spell a request body of its own: no free-form body literal, no quoted wire-name key, and no wire name of a field the language renames |
| 2 | `tools/sdkgen`: `TestEveryRequestRowIsReachedByEverySDK` | every request row the contract puts in the SDK contract is built by every SDK |
| 3 | `kernel/restapi`: `TestTheRequestRowsAgreeWithTheDaemonsRequestBodies` | the contract's request rows and the daemon's request structs name the **same** fields — in both directions |
| 4 | the golden fixture + three SDK suites | the three SDKs serialize the same row to the same bytes |
| 5 | the TypeScript compiler | the SDK's own `npm test` runs `tsc`, so the client's row literal is checked against the generated row |

Guard 3 is the one that found the `correlation_id` bug: the daemon read a
field the row did not name, and the row named a field the daemon ignored.

## 4. The mutations

Four mutations, four caught, all reverted:

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the Python client builds a request body of its own again | the SDK ratchet — *"sdk/python/agezt/client.py spells the request field "text" on the wire"* and *"spells the wire name "from" of a field it calls "from_""* |
| M2 | the TypeScript client spells the renamed field's wire name by hand | the SDK ratchet — *"sdk/typescript/src/client.ts spells the wire name "reply_to" of a field it calls "replyTo""* |
| M3 | the daemon's request struct gains a field the contract does not name | the daemon ratchet — *"the "mailbox_send" endpoint reads "priority", which the contract's "mail_draft" row does not name"* |
| M4 | the contract's row renames a field the daemon reads | the daemon ratchet — *"the contract's "mail_draft" row names "correlation", which the "mailbox_send" endpoint does not read"* |

## 5. Preserved behaviour

- **The wire protocol did not change.** The SDKs send the same bytes as
  before: the sparse rule is the rule the clients already followed by hand.
- **The clients' public API did not change**, except for additions: the
  Python SDK exports `RunRequest` and `MailAck` now (the TypeScript and Rust
  SDKs already exported them), and the TypeScript catalog exports the
  `toWire` functions and the local-name maps.
- **The daemon did not change.** The same struct fields, the same reads; the
  contract caught up with it.
- **The browser did not change.** This slice is the `/api/v1` surface; the
  console's `/api/*` surface is W5.8's and W5.9's.

## 6. Verification

- `gofmt -l kernel cmd plugins tools sdk` — clean.
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — green.
- `go run ./tools/archcheck` — clean (340 packages, 0 violations).
- `go run ./tools/deadcodecheck` — clean (4 cross-package test seams
  allowlisted: the contract's row renderer, which only kernel/restapi's
  golden test consumes).
- `go run ./tools/depscheck` — clean.
- `go run ./tools/sdkgen -check` — in sync.
- `go run ./tools/structure-md -check -out .project/STRUCTURE.generated` — in
  sync.
- `go run ./tools/openapi -check` — in sync (the console's document did not
  move).
- `go run ./tools/changelog-lint` — clean.
- `go run ./tools/docclaimscheck -base origin/main` — clean.
- `go run ./tools/docsindex` — clean.
- Python SDK: `python -m unittest discover -s tests` — **51** tests, green.
- TypeScript SDK: `npm test` (tsc + node:test) — **31** tests, green.
- Rust SDK: `cargo fmt --check` + `cargo test` — clean, **39** tests, green.

Measured: the SDKs' hand-written request bodies **8 → 0** (Python **4 → 0**,
TypeScript **4 → 0**); the request bodies built from the contract's rows
**3 → 11** (Python 4, TypeScript 4, Rust 3); the fields the daemon reads and
the contract cannot see **1 → 0**.
