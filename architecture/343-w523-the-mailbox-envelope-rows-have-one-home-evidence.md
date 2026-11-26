# W5.23 — the mailbox's envelope rows have one home: the surface serves the contract's rows

## 1. What was wrong

W5.22 gave the board's **message row** one home: the contract owns `Mail` (the
wire row) and `Draft` (the request row), and the REST surface serves and
decodes those rows instead of spelling their fields.

The **answers around the messages** were left behind. The mailbox surface wrote
its six envelopes by hand — one `map[string]any` per endpoint — and decoded its
ack body from a hand-written struct:

| home | row | fields | written as |
|---|---|---|---|
| `kernel/restapi` — the list answer | `mailbox_messages` | messages, count | a hand-written map |
| `kernel/restapi` — the send's answer | `mailbox_sent` | message, correlation_id | a hand-written map |
| `kernel/restapi` — the replies answer | `mailbox_replies` | id, replies, count | a hand-written map |
| `kernel/restapi` — the inbox answer | `mailbox_inbox` | name, waiting, count | a hand-written map |
| `kernel/restapi` — the ack answer | `mail_acked` | acked, id, by | a hand-written map |
| `kernel/restapi` — the topics answer | `mailbox_topics` | topics | a hand-written map |
| `kernel/restapi` — the ack body | `mail_ack` | by | a hand-written struct |

**Fifteen** field names, spelled in **seven** places, in an *implementation*
package — while the contract declared the same six envelopes and the same ack
body in `restapiapi` for the SDKs. So each envelope had two answers: the
contract's and the surface's, and nothing said they were the same list.

The failure is the familiar one: the surface renames `waiting` to `pending` and
the SDKs keep reading `waiting`. Nothing fails, because nothing ever said the
two lists were the same list.

## 2. What was built

### 2.1 The rule

**The rows the daemon sends live in the contract**, next to the message row
they carry — `kernel/contract/boardapi`. The surface serves them:

```go
// kernel/contract/boardapi/boardapi.go
type MailboxList struct {
	Count    int   `json:"count"`
	Messages []Mail `json:"messages"`
}
```

and the mailbox surface has **no** hand-written view of a row left:

```go
// kernel/restapi/mailbox_handlers.go
writeJSON(w, http.StatusOK, boardapi.MailboxList{Count: len(views), Messages: views})
```

The ack body moved with them — it is the one row the mailbox surface *reads*,
besides the draft:

```go
type Ack struct {
	By string `json:"by"`
}
```

The fields are declared in the order the wire speaks them (alphabetically), for
the same reason `Mail`'s are: the field order **is** the member order of the
served JSON, and the golden fixtures pin it. The served bytes did not move.

### 2.2 The two guards

**A — the stable-subset ratchet** (`kernel/contract/restapiapi/boardrows_test.go`).
The contract's envelope rows are the **stable subset** of the rows the daemon
serves: every field the contract names must be on the served row, under the
same name **and** a type the contract allows. The wire row may carry more —
that is the rule, not a drift — and one envelope does: the send's answer adds
the `correlation_id` the board event carries, which the contract's row does not
name.

The contract already knew the mail row's *types*: `TypeMail` and `TypeMailSlice`
were in the vocabulary before this slice, naming a shape nobody pointed at. Now
they point at `boardapi.Mail` and `[]boardapi.Mail`, so an envelope that
carries the wrong kind of message fails.

**B — the surface's own guard** (`kernel/restapi/mailbox_rows_test.go`). The
stable-subset ratchet holds the *contract* and the *row* together, but nothing
stopped the surface from going back to a hand-written map. So the surface is
measured: **a map literal that spells the field names of a mailbox row fails**,
and a **declared exception that no longer matches fails too** — a stale
exception is a lie about the code.

There is exactly one declared exception, and it is a row that is deliberately
not the mail row:

```
mailbox_watch.go: from,help,id,to,topic
    the watch stream's fallback frame: the event's metadata view for a message
    that was evicted from the store, not the mail row (the body lives in the
    store)
```

## 3. What was measured

| | before | after |
|---|---|---|
| hand-written views of a mailbox row in the surface | **7** | **0** |
| hand-spelled field names in the surface | **15** | **0** |
| rows the contract declares and the surface sends, checked | **1** (the mail row) | **7** |
| a renamed envelope field | the SDKs keep the old name | the stable-subset ratchet fails |
| a hand-written map comes back | nothing says so | the surface's guard fails |
| the served bytes | — | **unchanged** (the golden fixtures are byte-identical) |

The **before** side is not a different tree: it is the *same* tree with the same
seven hand-written spellings, and nothing that checked them.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w523_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the contract renames an envelope field.** `RowMailboxInbox`'s `waiting`
becomes `pending`:

```
the contract's "mailbox_inbox" row names "pending", which the row the daemon serves does not carry
```

**M2 — the contract calls a field a type the served row does not carry.**
`RowMailboxTopics`'s `topics` becomes a free-form object:

```
the contract's "mailbox_topics" row names "topics" as "any", the row the daemon serves carries it as map[string]int
```

**M3 — the served row drops a field the contract names.** `boardapi.MailAcked`
loses its `by` field (and the call site with it):

```
the contract's "mail_acked" row names "by", which the row the daemon serves does not carry
```

**M4 — the surface writes the envelope by hand again.** The list answer goes
back to a map literal:

```
mailbox_handlers.go writes a hand-written view of the "mailbox_messages" row: it spells count,messages
```

**M5 — the declared exception goes stale.** The watch's fallback frame is
served from the event payload instead of a hand-written map, so the exception
no longer matches anything:

```
mailbox_watch.go declares an exception for "from,help,id,to,topic" that no longer spells a row by hand — the exception is stale and must be deleted in the same change
```

## 5. What is untouched

- **No published artifact moved.** The SDK catalogs and the generated OpenAPI
  catalog are byte-identical (`sdkgen -check`, `openapi -check`), and the REST
  mailbox's golden fixtures are byte-identical to `main`'s — the served bytes
  are the same bytes.
- **No layer rule moved.** `archcheck` still reports 0 violations; the board
  contract gained rows and imports nothing new.
- **The message row did not move.** `boardapi.Mail` and `boardapi.Draft` are
  exactly as W5.22 left them.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`.

## 6. What remains open

- **The REST surface still writes six rows by hand**: the health answer, the
  run result (in three places), the run arc, the artifacts answer and the
  update answer. They are the same disease and the same cure — the row moves
  into the contract and the surface serves it — but each one has a wrinkle the
  mailbox rows did not have: the run result's **failure** answer carries an
  `error` field and no `answer`, and the run arc carries the daemon's own event
  type, so the contract's row and the row the daemon sends are not the same
  set. Those are decisions, not transcriptions.
- **The failure answer and the SSE frames are the daemon's own shapes.** The
  contract declares no row for them, so they stay hand-written until it does.
- **The board tool's own view is its own shape** (it renders the *stored* row
  into the tool's view, with `at` instead of `ts_unix_ms`), and it keeps its own
  ratchet in `plugins/tools/boardtool/wire_test.go`.
