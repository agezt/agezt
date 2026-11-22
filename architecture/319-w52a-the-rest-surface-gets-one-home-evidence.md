# 319 — W5.2a: the `/api/v1` surface gets one home

**Slice:** W5.2a of the W program (`architecture/21-migration-roadmap.md`).
**Date:** 2026-11-19.
**Branch:** `main`.

---

## 1. What was measured before anything was written

The daemon serves two HTTP surfaces. The console's internal one (`/api/*`) is
owned by `kernel/contract/opapi` and was given its own catalogue in W5.1: the
OpenAPI document and the generated browser client both read that table, and a
route the catalogue does not name cannot be registered.

The **other** surface had no home at all. It is the one the Python, TypeScript
and Rust SDKs speak:

| Where the `/api/v1` surface was written | How |
|---|---|
| `kernel/restapi/restapi_routes.go` | **16** `router.Handle` calls, **16** path literals |
| `sdk/python/agezt/client.py` | **13** hand-written path literals, 2 hand-written wire rows |
| `sdk/typescript/src/client.ts` | **15** hand-written path literals, 6 hand-written wire rows |
| `sdk/rust/src/client.rs` | **16** hand-written path literals, 6 hand-written wire rows |

Nothing tied those copies together. The only thing that noticed a drift was
`tools/sdkparity`, and it could not notice much: it extracted the daemon's
routes with a regex over `kernel/restapi/restapi*.go`
(`\.Handle(?:Func)?\("(/api/v1/[^"]*)"`), scraped each SDK's sources for the
same strings, and wrote a report. Its `-check` failed **only when the report
was stale** — never because an SDK lacked a route. The report itself said so:

> *"This report statically compares daemon `/api/v1` route registrations against
> route strings used by the Go, Python, TypeScript, and Rust SDK source trees.
> It is a coverage signal, not a behavioral conformance test."*

So the surface had **four** homes — the daemon's and three SDKs' — and the gate
between them measured the *age of a document*.

---

## 2. What the slice does

### 2.1 The contract — `kernel/contract/restapiapi` (767 lines)

A new contract package holds the surface as **data**:

- **`Routes()`** — 16 routes: path, tier (public/user/admin), kind
  (json/sse/text/bytes), verbs, query parameters, body policy (mutation flag,
  body cap), and the probes (`/healthz`, `/readyz`, `/metrics`).
- **`Endpoints()`** — 15 operations an SDK can call: verb, path template
  (`/api/v1/mailbox/messages/{id}/ack`), request row, response row, query.
  **11** of them are marked `SDK: true`; the four that are not (artifact
  transfer, self-update) say so in the contract instead of going missing — the
  silence is then a decision, not a hole.
- **`Rows()`** — 17 wire rows: field name, type, optionality, and the
  per-language local name where the wire name is a keyword (`from` → `from_` in
  Python) or where a language has its own spelling (`reply_to` → `replyTo` in
  the TypeScript draft row).

Two rules keep the table honest, both written into the package's doc comment:

1. **A row is the stable subset of a payload.** The server may add fields (the
   mailbox view adds `acked_by`); SDKs ignore unknown fields, so a row names
   only the fields every SDK already speaks. This is the rule
   `docs/EVENT-SCHEMA.md` states for events.
2. **A path template uses `{name}`** for a segment the caller supplies. The
   daemon registers the prefix; the SDK substitutes the segment.

### 2.2 The daemon registers FROM the contract

`kernel/restapi`'s `Handler` no longer holds sixteen registrations. It holds a
**map from route id to handler** and walks the table:

```go
for _, r := range restapiapi.Routes() {
    h, ok := handlers[r.ID]
    if !ok {
        panic("restapi: contract route has no handler: " + r.ID)
    }
    delete(handlers, r.ID)
    router.Handle(r.Path, routeOpts(r), h)
}
if len(handlers) != 0 { … panic("restapi: handler has no contract route: " + id) }
```

A route in the contract without a handler, or a handler without a contract
route, **panics at boot** rather than answering 404 on a path every SDK was
told to call.

The transport policy moved with it: `routeOpts` derives the tier, the verb
list, the body cap and the mutation flag from the route row, and the admin
tier's own 401 wording is applied there.

### 2.3 `tools/sdkgen` — the generator

One command renders the contract into each SDK's language:

| File | What it holds |
|---|---|
| `sdk/python/agezt/_generated.py` (201 lines) | `Paths` + the wire rows — dataclasses for the rows Python models as classes (`RunResult`, `Mail`, `MailDraft`), TypedDicts for the rest |
| `sdk/typescript/src/generated.ts` (141 lines) | `PATHS` + one interface per wire row |
| `sdk/generated.go` (30 lines) | the board row's wire field names, **derived by reflection from `kernel/app/board`'s own structs** |

The Go file deserves its own note: the Go SDK speaks the **control plane**,
not REST — it has no REST client and no REST paths. What it shares with the
other SDKs is the board's wire vocabulary, and that vocabulary has an owner:
the app layer's `board.Message` and `board.SendInput` structs. The generator
reads their json tags, so a renamed field changes the constants with it. The
constants are then used by the Go SDK's `parseMail` and `SendMail`.

The Go output is passed through `go/format`, so the repository's format gate
does not depend on how the template happens to be spaced.

### 2.4 The SDKs use the generated names

- **Python**: `client.py` imports `Paths`, `Mail`, `RunResult`, `Health`,
  `Models`, `RunArc` from the generated catalog; the two hand-written
  dataclasses are gone; the client's own annotations now name the generated
  rows. The package's public surface grew the row types (`Health`, `Models`,
  `RunArc`, `MailDraft`) and kept every existing name.
- **TypeScript**: `client.ts` imports `PATHS` and the row interfaces;
  `index.ts` re-exports the generated types and `PATHS`. `StreamEvent` stays
  hand-written — an SSE frame is not a JSON row.
- **Go**: `sdk/mailbox.go` reads and writes the generated field names.

### 2.5 `tools/sdkparity` is retired

The tool (three files, **693** lines), the report (`docs/SDK-PARITY.md`, 55
lines) and the freshness gate are gone. `make sdk-parity` becomes
`make sdk-gen-check`; the CI job that ran the parity check now runs the
generator's sync check.

---

## 3. The guards

**Five gates, in three packages:**

| Gate | What it fails on |
|---|---|
| `kernel/restapi/restapi_contract_test.go` | the daemon's registered routes (path, verbs, tier, body cap, mutation flag) differ from the contract — **in either direction** |
| `kernel/contract/restapiapi/restapi_test.go` | duplicate route ids/paths/endpoint ids/row fields; an endpoint naming an unknown route or row; an SDK endpoint that answers no row and is not an SSE stream |
| `tools/sdkgen/main_test.go` — *in sync* | a generated catalog that does not match the contract |
| `tools/sdkgen/main_test.go` — *no path of its own* | an SDK source outside the generated catalog that spells a REST path |
| `tools/sdkgen/main_test.go` — *every path reached* | a contract path in the SDK contract that an SDK does not reach through the generated constant |
| `tools/sdkgen/main_test.go` — *the wire projection agrees* | a field of the contract's message rows that the app layer's board projection does not carry |

**Four mutations, four catches** (each reverted immediately):

| Mutation | What fired |
|---|---|
| **A** — the contract names a route the daemon does not serve | `panic: restapi: contract route has no handler: rogue` |
| **B** — an SDK source spells a REST path of its own | `sdk/python/agezt/client.py spells the REST path "/api/v1/health" of its own` |
| **C** — an SDK stops reaching a contract path | `the typescript SDK never calls "/api/v1/mailbox/topics" (PATHS.mailboxTopics is missing)` |
| **D** — the generator drops a path from the catalog | `sdk\typescript\src\generated.ts is stale — run go run ./tools/sdkgen` |

The harness itself had a defect, and the SDK suites caught it: mutation C's
revert replaced the **first** occurrence of the constant it put back, which
was the *inbox* call, not the topics call — so the two TS methods swapped
paths. The TypeScript SDK's own suite failed on it
(`mailboxReplies / mailboxMessages / mailboxTopics map their shapes`), which is
the point: the mutation was reverted by hand, and the hand-written revert was
wrong in a way only a real client test could see. The fix was made, and the
suite passes (23/23).

---

## 4. The measurement

| Measure | Before | After |
|---|---:|---:|
| `router.Handle` calls in the daemon's route table | **16** | **1** (a loop) |
| path literals in `kernel/restapi/restapi_routes.go` | **16** | **0** |
| hand-written path literals in the Python client | **13** | **0**¹ |
| hand-written path literals in the TypeScript client | **15** | **0** |
| hand-written wire rows in the Python / TypeScript SDKs | 2 / 6 | **0 / 0** (generated) |
| places the SDK surface is written | 4 (daemon + 3 SDKs) + the Go SDK's copy | **1** (the contract) + generated catalogs |
| the retired tool + report | **748** lines | **0** |
| the gate that could detect a missing SDK route | none | 5 ratchets |

¹ one mention remains in the Python module docstring, which is prose, not a
path.

**The gates, all green:** `gofmt`, `go build ./...`, `go vet ./...`,
`go test ./...`, `archcheck` (341 packages, 1 allowlisted import violation),
`deadcodecheck` (33 public SDK findings + 1 cross-package seam, no new ones),
`depscheck` (24 dependencies), `sdkgen -check`, `structure-md -check`,
`changelog-lint`, `docclaimscheck`, `docsindex`, `staticcheck` 2026.2.1
(`GOTOOLCHAIN=go1.27.1`), frontend `typecheck` and `knip`.

The full `go test ./...` run surfaced two failures — `kernel/warden`
(`TestRun_TimeoutKillsAndFlagsTimedOut`) and `plugins/channels/irc`
(`TestSessionRegistersJoinsAndReplies`) — that **pass in isolation** and are
timing/port flakes under the parallel full-suite load on this machine; neither
package is touched by this slice.

The SDK suites: the Python SDK's **40** tests pass, the TypeScript SDK's
**23** tests pass, and the Go SDK's suite passes.

The frontend suite is **untouched by this slice** (`git status frontend/` is
empty), and it is flaky under load: the same clean-HEAD worktree measured
**26 failed files / 88 failed tests** on this machine right after this tree
measured **25 / 90**, and the W5.1 baseline from the same day was **24 / 83**.
The numbers move with the machine's load (the failures are 5-second timeouts
and the `localStorage` class); nothing in the frontend was changed here.

---

## 5. What is left

1. **The Rust SDK joins the catalog.** The generator's Rust emitter was written
   and then **removed from this slice on purpose**: this machine has no Rust
   toolchain, so a generated Rust file could not be compiled or `cargo fmt`-ed
   here, and shipping unverified code is worse than shipping it a slice later.
   W5.2b re-adds the emitter and wires `sdk/rust/src/client.rs` to it.
2. **Server-produced golden fixtures.** The SDK tests still assert against
   hand-written payloads. The contract now makes the fixture story possible:
   the daemon's own handlers can answer the contract's endpoints, and the
   answers can be recorded as the golden files every SDK test consumes.
3. **The self-validating `contract/fixtures`.** `contract/fixtures/*.json` are
   hand-written payload examples, and `fixtures_test.go` checks each file's
   *own* shape — it never checks that any emitter produces it. Those fixtures
   retire with the golden ones.
4. **The message row still has two homes.** The REST contract's `mail` row and
   the app layer's board projection (`kernel/app/board`'s `Message`) name the
   same fields, and a ratchet ties them — but one of them is a copy. The
   honest end state moves the client-visible row down into `boardapi` and
   derives both from it. That is a small, separate slice.

---

## 6. The files

**New**

- `kernel/contract/restapiapi/restapi.go` — the contract (767 lines)
- `kernel/contract/restapiapi/restapi_test.go` — the contract's invariants
- `kernel/restapi/restapi_contract_test.go` — the daemon ⇄ contract ratchet
- `tools/sdkgen/main.go` — the generator
- `tools/sdkgen/main_test.go` — the four ratchets
- `sdk/python/agezt/_generated.py`, `sdk/typescript/src/generated.ts`,
  `sdk/generated.go` — the generated catalogs

**Changed**

- `kernel/restapi/restapi_routes.go` — registers from the contract
- `sdk/python/agezt/client.py`, `sdk/python/agezt/__init__.py`
- `sdk/typescript/src/client.ts`, `sdk/typescript/src/index.ts`
- `sdk/mailbox.go`
- `Makefile`, `.github/workflows/ci.yml`, `.gitattributes`
- `architecture/00-README.md`, `architecture/01-daemon-boot-cmd-agezt.md`,
  `architecture/12-sdks-and-contract.md`, `architecture/21-migration-roadmap.md`
- `docs/API-STABILITY.md`, `docs/COMPARISON.md`, `docs/EVENT-SCHEMA.md`,
  `docs/REFACTORING-INDEX.md`, `docs/index.md`, `README.md`
- `CHANGELOG/unreleased/current.md`

**Deleted**

- `tools/sdkparity/` (3 files, 693 lines), `docs/SDK-PARITY.md` (55 lines)
