# 321 — W5.2c: the Rust SDK joins the catalog

**Slice:** W5.2c of the W program (`architecture/21-migration-roadmap.md`).
**Date:** 2026-11-21.
**Branch:** `main` (parent: `890c44fd`, "W5.2b: the server's own answers become the fixtures").

---

## 1. What was measured before anything was written

W5.2a gave the `/api/v1` surface one home and W5.2b gave the wire's answers
one home. One SDK was still outside both: **Rust**.

W5.2a had removed the Rust emitter on purpose, with a written reason: *"this
machine has no Rust toolchain"*. **That reason was wrong.** The toolchain is
installed — `rustup` 1.93.0, `cargo` 1.93.0, `rustc` 1.93.0, `rustfmt` 1.8.0 —
it is simply **not on `PATH`** in the shell the agent runs in:

```
$ Get-Command cargo,rustc,rustup     # nothing
$ & "$env:USERPROFILE\.cargo\bin\cargo.exe" --version
cargo 1.93.0 (083ac5135 2025-12-15)
```

With `$env:PATH = "$env:USERPROFILE\.cargo\bin;$env:PATH"` the existing Rust
SDK builds, formats and tests cleanly. The slice was never blocked — it was
parked on a false premise.

What the Rust SDK actually looked like before this slice:

| Where the Rust SDK spelled the contract | How |
|---|---|
| `sdk/rust/src/client.rs` | **12** hand-written path literals |
| `sdk/rust/src/client.rs` | **6** hand-written wire structs (`Health`, `Models`, `RunResult`, `RunArc`, `Mail`, `MailDraft`) |
| `sdk/rust/src/client.rs` | **57** hand-written wire-name literals across **27** names |
| `sdk/rust/src/generated.rs` | did not exist — W5.2a had deleted it |

---

## 2. What the slice does

### 2.1 The generator — `tools/sdkgen/render_rust.go`

The Rust SDK has **no runtime dependency at all**: standard library only, with
its own HTTP/1.1 client and its own JSON codec. So the generated Rust cannot
lean on serde; it leans on the crate's own `json::Value`:

```rust
impl Mail {
    pub fn from_value(v: &Value) -> Self {
        Self {
            id: v.str("id").unwrap_or("").to_string(),
            topic: v.str("topic").unwrap_or("").to_string(),
            text: v.str("text").unwrap_or("").to_string(),
            ...
        }
    }

    pub fn to_value(&self) -> Value { ... }
}
```

`from_value` reads the wire names; `to_value` writes them. That is what moved
the wire vocabulary out of `client.rs` and into the catalog: the client no
longer says what a message looks like, it only says where to send it.

Two decisions worth naming:

1. **The types stay flat.** A missing field reads as its zero value — an empty
   string, zero, false, an empty list — never an `Option`. The Rust SDK's
   public API has always been flat (`Health.version` is a `String`), and the
   contract's own rule about the daemon's answers says the same: a row names
   the fields every answer carries, and the daemon may add fields the SDK
   ignores.
2. **An optional field is omitted when it is empty.** `to_value` writes a
   required field always and an optional one only when it holds something. An
   absent optional field and an empty one are not the same request — and this
   reproduces exactly what the hand-written client used to send.

### 2.2 The catalog — `sdk/rust/src/generated.rs` (696 lines)

`paths` — ten constants, one per contract path, plus `paths::ALL` — and
**seventeen** structs, one per wire row. `lib.rs` re-exports them, so the
crate's public API keeps its names (`Health`, `Mail`, `MailDraft`, …) and
gains the rows it did not have.

The one name that needed a decision: the `api_error` row's `type` field is a
Rust keyword. Rather than spelling `r#type` in the catalog, the **contract**
declares the local name (`Names: {"rust": "kind"}`) — which is also the name
the SDK already used (`Error::Api`'s field). The generator still escapes a
reserved word with `r#` if a future row needs it.

### 2.3 The client — `sdk/rust/src/client.rs` (609 → 465 lines)

Twelve path literals became `paths::…` constants; six hand-written structs
became the generated rows; the hand-written `mail_from`, `mails_from` and
`str_field` helpers became `from_value`.

Two wire names the client still spells, and why they are not the contract's
fault:

| Literal | Where | Why |
|---|---|---|
| `"error"` | the error **envelope**'s key | the `api_error` row describes the object *inside* the envelope (`{type, message}`), not the wrapper |
| `"status"` | the failed-run body | that body (`{correlation_id, model, status, error}`) has no contract row; its `status` is the `run_result` row's field, read from another body |

### 2.4 The fixtures — `sdk/rust/tests/golden.rs` (4 tests)

The Rust SDK parses the daemon's **own** recorded answers with its own types:
`Health`, `Models`, `RunResult`, `RunArc`, `MailboxMessages`, `MailboxSent`,
`MailAcked`, `MailboxReplies`, `MailboxInbox`, `MailboxTopics`. It also
checks that the SDK's path constants and the server's path templates are one
set.

### 2.5 The ratchets

`sdkgen`'s two coverage ratchets now cover Rust as well: no Rust source
outside the catalog may spell a REST path, and every contract path must be
reached through the generated constant.

The coverage check also got **stricter for everyone**: the reference is now
matched as a whole identifier, so a client that calls only `paths::RUN_ARC`
no longer looks like one that also calls `paths::RUN` (the same held for
`Paths.RUN` in Python).

### 2.6 The formatter is part of the contract

`cargo fmt --check` covers `src/generated.rs` like every other file in the
crate — so the generator must emit the shape `rustfmt` produces, not merely a
shape it tolerates. Two rules had to be learned and implemented:

- **`fn_call_width` is 60, not `max_width`'s 100.** A call's *arguments* must
  fit in 60 columns; a longer argument list breaks into one argument per line
  even when the whole call would fit. The first attempt emitted
  `m.insert("default_model".to_string(), Value::Str(self.default_model.clone()));`
  (85 columns, perfectly valid Rust) and `cargo fmt --check` rejected it.
- **A block's body is indented by four.** The optional-field branch needed
  the insert at twelve columns, not eight.

---

## 3. The measurement

| Measure | Before | After |
|---|---|---|
| hand-written path literals in the Rust client | 12 | **0** |
| hand-written wire-name literals in the Rust client | 57 (27 names) | **21** (8 names — all in doc comments, the SSE frame name, the error envelope key and the tests) |
| hand-written wire structs in the Rust client | 6 | **0** |
| wire rows the Rust SDK knows | 6 | **17** |
| generated SDK catalogs | 3 | **4** |
| SDKs the coverage ratchets cover | 2 | **3** |
| Rust SDK tests | 13 + 15 + 3 | **17 + 15 + 3** (+4 golden) |
| `client.rs` | 609 lines | **465 lines** |

---

## 4. The mutations

Three mutations, each reverted, each caught.

### A — the Rust client spells a path of its own

`sdk/rust/src/client.rs`: `self.get_json(paths::MAILBOX_TOPICS)` →
`self.get_json("/api/v1/mailbox/topics")`.

`TestTheSDKsNameNoPathOfTheirOwn` →
*`sdk/rust/src/client.rs spells the REST path "/api/v1/mailbox/topics" of its own`* (**FAIL**).

### B — the Rust SDK's parser drifts from the server

`sdk/rust/src/generated.rs`: `Mail::from_value` reads `v.str("body")` instead
of `v.str("text")`.

- `cargo test --test golden` → `the_mailbox_rows_parse` panicked:
  `left: ""`, `right: "golden message"` (**FAIL**)
- `go run ./tools/sdkgen -check` → `sdk\rust\src\generated.rs: stale` (**FAIL**)

### C — the daemon's wire shape changes

`kernel/restapi/restapi_routes.go`: the health answer's `default_model` field
renamed to `model_name`.

1. `TestTheGoldenFixturesAreInSync` → `sdk/fixtures/health.json is stale`
   (**FAIL**)
2. After `AGEZT_UPDATE_GOLDEN=1` (the fixture now holds the **new** bytes):
   - Rust: `the_health_and_model_rows_parse` panicked: `left: ""`,
     `right: "model-a"` (**FAIL**)
   - Python: `KeyError: 'default_model'` (**FAIL**)

**A first attempt at C was itself instructive.** The mutation first *added* a
field (`model_name`) instead of renaming one — and the Rust suite stayed
**green**. That is correct behaviour, not a hole: the contract's rule says the
daemon may add fields and the SDKs ignore what they do not know. A mutation
that adds a field proves nothing, so it was replaced by the rename.

Each mutation was reverted and re-verified green (`cargo test`, `cargo fmt
--check`, `go run ./tools/sdkgen -check`, `go test ./kernel/restapi/`).

---

## 5. The gates

Go: `gofmt -l` clean, `go build ./...` clean, `go vet ./...` clean,
`go test ./...` green. `archcheck`, `deadcodecheck`, `depscheck`,
`sdkgen -check`, `structure-md -check`, `changelog-lint`, `docclaimscheck`,
`docsindex`, `staticcheck 2026.2.1` — all green.

SDKs: Python **47/47** (7 golden), TypeScript **27/27** (4 golden), Rust
**15 + 17 + 3** (4 golden) with `cargo fmt --check` clean.

Frontend: unchanged by this slice (`typecheck` and `knip` still green; the
vitest suite's baseline is the one measured in W5.2b).

---

## 6. What is deliberately left

- **The message row's second home** (carried from W5.2a): the REST contract's
  row and the app layer's board projection name the same fields. A ratchet
  ties them, but one is a copy; the honest end state moves the client-visible
  row down into `boardapi`.
- **The Go SDK's control plane** (NDJSON) has no golden fixture yet. The
  REST surface is covered by all three REST SDKs; the control plane is not.
- **The error envelope and the failed-run body** have no contract rows (§2.3),
  so the Rust client still spells `"error"` and `"status"` there. Adding rows
  for them would be the next honest step — and it would touch every SDK.
