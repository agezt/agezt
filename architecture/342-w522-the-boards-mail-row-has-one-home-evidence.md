# W5.22 — the board's mail row has one home: the contract owns the wire row and the request row

## 1. What was wrong

The board's **mail row** is the row the daemon serves on the wire when a client
asks for messages: `id`, `topic`, `text`, `from`, `to`, `reply_to`, `help`,
`ts_unix_ms`, `acked_by`. The board's **mail-draft row** is the row a client
*sends* to leave a message: `text`, `from`, `to`, `topic`, `reply_to`, `help`,
`correlation_id`.

Both rows were spelled out by hand in **several** places, and each place was its
own answer to "what are the mail row's fields?":

| home | row | fields | written as |
|---|---|---|---|
| `kernel/contract/restapiapi` — `RowMail` | response | 8 | a declaration for the SDKs |
| `kernel/app/board` — `Message` | response | 9 | json tags + a hand-written `project()` |
| `kernel/restapi` — `mailMsgView` | response | 9 | a hand-written `map[string]any` |
| `tools/sdkgen` → the four SDKs | response | 9 | derived from the **app layer's** copy |
| `kernel/contract/restapiapi` — `RowMailDraft` | request | 7 | a declaration for the SDKs |
| `kernel/app/board` — `SendInput` | request | 7 | json tags |
| `kernel/app/board` — `SendRequestInput` | request | 7 | json tags |
| `kernel/restapi` — `mailboxSendRequest` | request | 7 | json tags |

**Fifty-four** hand-spelled field names, and the response row alone had **three**
answers. Nothing said they were the same row, so nothing could say they had
drifted: the REST surface could rename `ts_unix_ms` to `ts_ms` while the SDK
catalogs still spoke `ts_unix_ms`, and the golden fixtures would have caught it
only because the fixtures are *bytes* — the row's **names** had no guard of their
own.

The SDKs made it worse in a way worth naming: `tools/sdkgen` derived the SDKs'
mail constants from the **app layer's** copy of the row. So the vocabulary of
four published SDKs rested on a struct that lived in an application package, and
the contract's own row (`RowMail`) was a *second*, unconnected declaration of
the same names.

## 2. What was built

### 2.1 The rule

**The row lives in the contract.** `kernel/contract/boardapi` owns both rows —
the one the daemon serves (`Mail`) and the one a client sends (`Draft`) — and
one pure function projects the stored row into the wire row (`MailOf`).

```go
// kernel/contract/boardapi/boardapi.go
type Mail struct {
	AckedBy  []string `json:"acked_by,omitempty"`
	From     string   `json:"from,omitempty"`
	Help     bool     `json:"help,omitempty"`
	ID       string   `json:"id,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
	Text     string   `json:"text"`
	To       string   `json:"to,omitempty"`
	Topic    string   `json:"topic"`
	TSUnixMS int64    `json:"ts_unix_ms"`
}

func MailOf(m Message) Mail { … }
```

The package stays **pure** (archcheck's principle P4): standard library and other
contract packages only — it is a row and a projection, no store and no I/O.

The app layer keeps its names, but as **aliases**, so every existing caller is
unchanged and no copy can grow:

```go
// kernel/app/board/board.go
type Message  = boardapi.Mail
type SendInput = boardapi.Draft
```

The REST surface decodes the draft row it asks for and serves the wire row it
promises — the hand-written `mailMsgView` map and the hand-written
`mailboxSendRequest` struct are **gone**.

### 2.2 The fields are in the order the wire speaks them

`Mail`'s fields are declared **alphabetically**, because the member order of a
Go struct **is** the member order of the JSON the daemon serves, and the REST
mailbox surface has always served them alphabetically (it built the row as a
`map[string]any`, and `encoding/json` sorts a map's keys). Declaring them in the
same order keeps the served bytes byte-identical — the golden fixtures did not
move.

### 2.3 The stable subset — the contract's own row, and the rule that binds it

`restapiapi.RowMail` stays: it is the row the **SDKs** speak, and the contract's
own law says a response row is the **stable subset** of the payload —

> a row is the STABLE SUBSET of a payload … the mailbox view adds `acked_by`

So the two rows are related, but not identically: the wire row may carry **more**
than the SDK row (`acked_by` is one), and the SDK row must carry everything the
contract names. That direction is the one that binds, so the new ratchet
(`kernel/contract/restapiapi/boardrows_test.go`) checks it:

- every field the contract's mail row names must be on the wire row, **under the
  same name**;
- **and** under a type the contract allows — a field the contract calls a string
  and the wire carries as a number is a field an SDK would misread.

The wire row may carry more; that is the rule, not a drift. So the ratchet is
deliberately **one-way**.

The request row is checked in **both** directions, and that check already
existed: W5.10's ratchet (`kernel/restapi/request_rows_test.go`) holds the
contract's request rows and the daemon's request structs to each other, and it
now reads `boardapi.Draft` — so a field the daemon reads and the contract does
not name, or a field the contract names and the daemon ignores, still fails.

### 2.4 The guards

Three guards hold the slice to its own question:

- **the stable-subset ratchet** (`kernel/contract/restapiapi/boardrows_test.go`)
  — the contract's mail row against the board's wire row, by name and by type;
- **the identity guard** (`kernel/app/board/contract_rows_test.go`) — the board's
  outputs carry the **contract's** row, by **identity**:

  ```go
  var _ func(ReadOutput) []boardapi.Mail = func(o ReadOutput) []boardapi.Mail { return o.Messages }
  ```

  The **function type** is deliberate: an *assignment* would also accept a copy
  with the same fields, but a function type requires identity — so a copy
  declared in the app layer **does not compile**.
- **the SDK derivation** (`tools/sdkgen`) — the SDKs' mail constants are derived
  from the contract's row, so a renamed wire field changes them with it, and
  `sdkgen -check` fails when the catalogs are stale.

## 3. What was measured

| | before | after |
|---|---|---|
| homes of the **response** row (independent spellings) | **3** | **1** |
| …plus the SDK-facing declaration (`RowMail`) | 1 (unconnected) | 1 (**checked** against the wire row) |
| hand-spelled names of the response row | 26 | **9** (+ 8 in the checked declaration) |
| homes of the **request** row (independent spellings) | **4** | **1** |
| …plus the SDK-facing declaration (`RowMailDraft`) | 1 (unconnected) | 1 (**checked**, both directions) |
| hand-spelled names of the request row | 28 | **7** (+ 7 in the checked declaration) |
| where the SDKs' mail constants come from | the **app layer's** copy | the **contract's** row |
| a renamed wire field | the SDKs keep the old name | `sdkgen -check` fails |
| a wire field the contract names but the wire does not carry | nothing says so | the stable-subset ratchet fails |

The **before** side is not a different tree: it is the *same* tree with the same
five hand-written spellings, and nothing that checked them.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w522_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the wire row renames a field the contract names.** `Mail.Topic` becomes
`Subject` (`json:"subject"`). The stable-subset ratchet says:

```
the contract's "mail" row names "topic", which the board's wire row does not carry
```

**M2 — the contract calls a field a string the wire carries as a number.**
`RowMail`'s `ts_unix_ms` becomes `TypeString`. The ratchet says:

```
the contract's "mail" row names "ts_unix_ms" as "string", the board's wire row carries it as int64
```

**M3 — the contract names a field the wire row does not carry.** `RowMail` gains
a `priority` field. The ratchet says:

```
the contract's "mail" row names "priority", which the board's wire row does not carry
```

**M4 — the app layer declares a copy of the wire row.** `type Message =
boardapi.Mail` becomes a nine-field struct, and `project` is rewritten to build
it — so **the package still compiles**, and only the identity guard refuses it:

```
kernel\app\board\contract_rows_test.go:19:86: cannot use o.Messages (variable of type []Message) as []boardapi.Mail value in return statement
```

**M5 — the wire row renames a field only the SDKs name.** `Mail.AckedBy`'s tag
becomes `acked` — a field the contract's row does not name, so the stable-subset
ratchet has nothing to say about it. The SDK catalog check does:

```
sdk\generated.go: stale (run "go run ./tools/sdkgen")
```

## 5. What moved on the wire — and what did not

**The REST mailbox's bytes did not move.** The served row is the same row, in
the same order: `sdk/fixtures/mailbox_*.json` are byte-identical to `main`'s
(`git status` shows no fixture touched), and the golden test
(`TestTheGoldenFixturesAreInSync`) is green.

Two things did move, and both are visible in the diff:

- **The control plane's board row now serves its members alphabetically.** The
  control plane served the app layer's struct, whose fields were declared in a
  *reading* order (`topic`, `text`, `ts_unix_ms`, `id`, …); the REST surface
  served the same fields alphabetically. A JSON object has no member order, so
  no contract moved — but the **bytes** of a pretty-printed control-plane board
  row differ from `main`'s. The alphabetical order won because it is the order
  the **pinned** fixtures and the SDKs' fixtures use.
- **The published OpenAPI for `/api/board/send` now says `text` is required.**
  The app layer's copy of the draft row had `json:"text,omitempty"`, and the
  catalog renders requiredness from the tags — so the published view said the
  body was optional while the daemon **rejects** an empty text, the REST
  contract's row said `Required`, and the SDKs already generated it as required.
  The single home says what the contract has always said.

The SDK catalogs changed in one place only: `sdk/generated.go`'s mail constants
are now in the row's alphabetical order. **Their values did not change**, and the
Python, TypeScript and Rust catalogs are byte-identical.

## 6. What is untouched

- **No layer rule moved.** `archcheck` still reports 0 violations; the contract
  package gained a row and a pure function and imports nothing new.
- **The stored row did not move.** `boardapi.Message` (the row the store holds)
  is unchanged, and the board tool's plugin — which reads the *stored* row —
  is unchanged.
- **The daemon's behaviour did not change.** The `Draft` row's `text` tag lost
  its `omitempty`, which changes no served byte (nothing marshals a draft) and
  only makes the published schema truthful.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, and on the browser side `tsc --noEmit`,
  `npm run build`, `npm run deadcode` (`knip` reports no issue).

## 7. What remains open

- **The board tool's own view is its own shape.** The board plugin renders the
  *stored* row into the tool's view (`tool_helpers.go`: `at` instead of
  `ts_unix_ms`, the body as it is), and that view is the tool's own vocabulary —
  the plugin may reach the contract and the platform layers, not the app layer's
  rows. It already has its own ratchet (`plugins/tools/boardtool/wire_test.go`
  pins the view's keys), and it is named here as out of scope rather than left
  silent.
- **The SSE watch's fallback stays a hand-written map.** When a message was
  evicted from the store between the write and the publish, the watch stream
  falls back to the *event's* metadata view (`id`, `topic`, `from`, `to`,
  `help`) — the body lives in the store, so that frame is deliberately not the
  mail row. Every other frame carries `boardapi.Mail`.
- **The contract's rows stay as declarations.** `RowMail` and `RowMailDraft`
  remain the SDK-facing vocabulary; they are now **checked** against the wire
  row rather than duplicated by it, which is the contract's own law (a response
  row is the stable subset of the payload) rather than this slice's invention.
