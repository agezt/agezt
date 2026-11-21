# 320 — W5.2b: the server's own answers become the fixtures

**Slice:** W5.2b of the W program (`architecture/21-migration-roadmap.md`).
**Date:** 2026-11-19.
**Branch:** `main` (parent: `d8a52f0e`, "W5.2a: the `/api/v1` surface gets one home").

---

## 1. What was measured before anything was written

W5.2a gave the `/api/v1` surface one home: the contract in
`kernel/contract/restapiapi`, from which the daemon registers its routes and
`tools/sdkgen` generates the SDK catalogs. One question that home made urgent
was still unanswered — **who says what the wire looks like?**

Two answers existed, and both were wrong in the same way:

| Where the wire shape was asserted | How |
|---|---|
| `sdk/python/tests/*.py` | each test **wrote its own payload** and called it a contract |
| `sdk/typescript/test/*.ts` | the same, in TypeScript |
| `contract/fixtures/*.json` (8 files) | hand-written event payloads |
| `contract/fixtures/fixtures_test.go` (114 lines) | checked **a fixture's own shape** |

The consequence of the first two rows: a renamed field or a changed envelope in
the daemon passed **every** SDK test and still broke **every** SDK. The SDKs
were asserting against themselves.

The consequence of the last two rows: `fixtures_test.go` never asked an emitter
to produce a payload, so it could not fail when the daemon changed. It was a
document about the past, with a test that said the document was shaped like a
document.

Only two of the eight fixtures had a consumer at all:

| Fixture | Consumer |
|---|---|
| `context_compacted_skill_rescue.json` | `frontend/src/lib/chat.test.ts` |
| `subagent_completed_async.json` | `frontend/src/lib/replay.test.ts` |
| the other **six** | none |

---

## 2. What the slice does

### 2.1 The producer — `kernel/restapi/golden_test.go` (365 lines)

The recorder drives the **real handlers** and keeps the bytes they answered:

- a **deterministic engine** — fixed correlation id (`corr-golden`), two fixed
  models (`model-a`, `model-b`), one fixed answer (`golden answer`), one fixed
  event (`evt-golden-1`);
- a **deterministic board** — fixed message ids (`m-1`, `m-2`, …), fixed
  timestamps (`1700000000000+`), two seeded messages (one direct, one
  broadcast), so the inbox, the replies and the topic counts all have something
  to answer;
- the **actual routes**, through `s.Handler().ServeHTTP`, with a bearer token.

Ten endpoints are recorded (`health`, `models`, `run`, `run_arc`, and the six
mailbox ones). Two gates:

```
go test ./kernel/restapi/ -run TestTheGoldenFixturesAreInSync     # the gate
AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures   # regenerate
```

The second test **skips** unless the environment asks for it, so an ordinary
test run never writes.

### 2.2 The artefacts — `sdk/fixtures/` (11 files)

`manifest.json` names, for every SDK-contract endpoint: the **id**, the verb,
the contract's path **template**, the concrete request the recorder made, the
status, the fixture's file — and, for the one endpoint without a fixture, the
reason.

The template is the string the SDKs' generated catalogs hold, so the manifest
is what lets an SDK check its own constant against the server's own table.

### 2.3 The consumers

| Consumer | Tests | What it checks |
|---|---|---|
| `sdk/python/tests/test_golden.py` | 7 | the path sets are one set; the server's bytes parse into the SDK's own types |
| `sdk/typescript/test/golden.test.ts` | 4 | the same, in TypeScript |

Both read the bytes and parse them with the SDK's **own** rows — `Mail.from_dict`,
`RunResult.from_dict` — not with a hand-written dict.

### 2.4 The retirement

`contract/fixtures/` is gone: eight hand-written payloads (80 lines), the
self-validating `fixtures_test.go` (114 lines) and the package itself — **194
lines**. The two fixtures the console actually reads moved next to their only
consumer:

```
contract/fixtures/context_compacted_skill_rescue.json → frontend/fixtures/context_compacted_skill_rescue.json
contract/fixtures/subagent_completed_async.json        → frontend/fixtures/subagent_completed_async.json
```

and the three frontend tests that read them now say `./fixtures/<name>` instead
of `../contract/fixtures/<name>`.

### 2.5 What has no fixture, on purpose

The mailbox **watch** (`/api/v1/mailbox/watch`) is an open-ended SSE stream.
A recorded frame would be a snapshot of a race rather than a contract, so the
manifest lists it with a note instead of a file, and every SDK's own suite
covers the stream's frames.

---

## 3. The measurement

| Measure | Before | After |
|---|---|---|
| self-validating fixture payloads | 8 | **0** |
| the Go test that validated a fixture's own shape | 114 lines | **0** |
| hand-written wire payloads the SDK tests asserted against | every SDK test | **0** |
| endpoints answered by the daemon and recorded as bytes | 0 | **10** |
| SDK tests that parse the **server's** bytes | 0 | **11** (7 Python + 4 TypeScript) |
| fixture files | 0 | **11** |

---

## 4. The mutations

Three mutations, each reverted, each caught.

### A — the daemon's wire shape changes

`kernel/restapi/restapi_routes.go`: the health answer's field `default_model`
renamed to `defaultModel`.

1. `TestTheGoldenFixturesAreInSync` →
   `sdk/fixtures/health.json is stale — regenerate the golden fixtures` (**FAIL**).
2. After `AGEZT_UPDATE_GOLDEN=1` (the fixture now holds the **new** bytes):
   - Python: `KeyError: 'default_model'` in `test_health_and_models` (**FAIL**)
   - TypeScript: **26/27**, the golden test fails (**FAIL**)

That is the whole point in two steps: the server's change is visible to the
SDKs because the fixture *is* the server's answer.

### B — the SDK's parser drifts from the server

`sdk/python/agezt/_generated.py`: `Mail.from_dict` reads `d.get("body")`
instead of `d.get("text")`.

- Python: `test_mailbox_send` → `AssertionError: '' != 'golden message'`; the
  SDK's own mailbox tests fail too (**FAIL**)
- `go run ./tools/sdkgen -check` → `sdk\python\agezt\_generated.py: stale`
  (**FAIL**)

### C — the manifest's path template drifts

`sdk/fixtures/manifest.json`: `run_arc`'s template `/api/v1/runs/{id}` →
`/api/v1/runs/{corr}`.

- `TestTheGoldenFixturesAreInSync` → `sdk/fixtures/manifest.json is stale` (**FAIL**)
- Python: `test_every_fixture_path_matches_the_generated_catalog` — *"the
  SDK's generated paths and the server's fixtures disagree"* (**FAIL**)
- TypeScript: *"the SDK's generated paths and the server's fixtures are one
  set"* (**FAIL**)

Each mutation was reverted by restoring the **full context** of the changed
line (not by replacing the first occurrence), and each was re-verified green
afterwards.

---

## 5. The gates

Go: `gofmt -l` clean, `go build ./...` clean, `go vet ./...` clean,
`go test ./...` green. `archcheck` (340 packages, 1 allowlisted import
violation), `deadcodecheck` (no unexpected dead code), `depscheck` (24 core
dependencies), `sdkgen -check`, `structure-md -check`, `changelog-lint`,
`docclaimscheck`, `docsindex` — all green. `staticcheck 2026.2.1` green.

SDKs: Python **47/47** (7 of them the new golden tests), TypeScript **27/27**
(4 of them the new golden tests).

Frontend: `typecheck` clean, `knip` reports no issues.

**The frontend suite and this machine.** The suite is red on this machine —
it was red before the slice and it is red after it. Measured on a clean
worktree of the parent commit (`d8a52f0e`, `git worktree add
D:\Codebox\PROJECTS\agezt-base4 HEAD`) against this tree, in the same
conditions:

| Tree | Failed test files | Failed tests |
|---|---|---|
| clean `d8a52f0e` (baseline) | 30 | 96 |
| this tree | 27 | 89 |

The three frontend test files this slice touched
(`src/lib/chat.test.ts`, `src/lib/replay.test.ts`,
`src/features/runs/lib/rundetail.test.ts`) are in **neither** failure list and
pass on their own: **3 files / 54 tests green**. The failures are the
machine's (this Node build has no `localStorage`, and the suite's 5-second
timeout is tight under load) — the baseline is *worse* than the tree, so the
slice cannot be the cause.

---

## 6. What is deliberately left

- **W5.2c — the Rust SDK.** It joins the generated catalog and reads the
  golden fixtures, but this machine has **no Rust toolchain** (`cargo` and
  `rustfmt` are missing). Writing Rust that cannot be compiled, formatted or
  tested here would be shipping unverified code; it is left to the next slice.
- **The Go SDK's control plane (NDJSON)** has no golden fixture yet. The REST
  surface is covered; the control plane is not.
- **The message row's second home** (carried over from W5.2a): the REST
  contract's row and the app layer's board projection name the same fields. A
  ratchet ties them, but one is a copy; the honest end state moves the
  client-visible row down into `boardapi`.
