# W5.9 — the registry's own request side gets its types

## 1. What was wrong

W5.8 gave the **browser's** request bodies their types: the console's route
catalogue has 70 routes that take a JSON body, and every property of every one
of those bodies now names the type its handler reads it as. What was left was
the registry's **own** surface — the operations a CLI or an SDK calls, the ones
no console route names.

Measured over the whole registry (324 operations, `.temp_files/w59_dump.json`):

| surface | before | after |
|---|---|---|
| operations with an untyped request property | **122** | **0** |
| untyped request properties | **418** | **0** |
| operations whose request body is fully typed | 202 | **324** |
| daemon packages carrying the change | — | **29** |

The 122 operations and their 418 properties sat in **29** packages:
approvals (3/8), artifacts (4/7), audit (5/12), autonomy (1/1), board (4/4),
budget (1/1), chat (1/2), edict (11/25), execprofile (1/1), files (4/7),
journal (6/13), memory (2/4), missioncontrol (1/2), okr (7/23), providers (1/1),
pulse (14/125), reaper (1/2), sandbox (2/3), seats (2/8), state (2/3),
steer (6/14), system (2/2), taste (3/8), tenants (4/4), tools (9/37),
update (1/4), webhook (2/5), workboard (21/89), world (1/3).

An empty property schema — `{}` — says "anything". So for **418** fields the
daemon's contract said nothing at all, and a mistyped field reached the
handler as a **silent default**: a text page size became the default page, a
number where a title belongs became an empty title, a text approval flag became
*false*, a null tool became an empty tool that was then drafted.

## 2. What was built

### 2.1 The rule: the field's read type

The rule is W5.8's, unchanged:

- the type is the field's **read** type — string, number, boolean, object,
  array;
- a **union** only where the handler genuinely accepts two meaningful shapes
  (the pulse approval flag: a boolean, or the text a form sends; the seats'
  model chain and tools: a list, or the one comma-separated string the reader
  splits; the workboard's tags, artifacts and criteria: the same two shapes);
- **no null and no junk arms**. A mistyped field is a usage error, not a
  default.

Every one of the 418 properties names its type now. The reader behind each
type is the handler's own: `requestText` reads a string, `requestNumber` and
`requestLimit` read a number, `journal.DecodeCursor` reads the text a journal
cursor is, `observationBool` reads a boolean, `forgeDecodeTool` reads the
object a forged tool is.

### 2.2 The door moved to the whole registry — and its message is the same

A mistyped body is refused at **admission**, before the handler runs and before
the audit sees it, and the refusal names the field and both types:

```
operation workboard_create input: $.max_attempts has wrong type: got string, want number
operation pulse_ask_resolve input: $.approve has wrong type: got number, want boolean|string
operation okr_keyresult input: $.target has wrong type: got null, want number
```

The dispatcher was **not** touched (the door was already there — the contract
says now what the door should check), and the wire protocol is unchanged: only
the *declared* type of a field moved. The presence of a field (absent vs null)
stays the handler's business.

### 2.3 What the callers send

The rule had to survive the real callers, so every type was checked against
them before it was written:

- the **CLI** (`cmd/agt`) sends numbers as numbers (`--priority`, `--limit`,
  `--max-attempts`, `--stale-after`, `--lease`), text as text, lists as lists
  (`--tag`, `--artifact`, `--criterion`), and the pulse approval as a
  **boolean**;
- the **Go SDK** (`sdk/`) sends the board's limit as a number, the inbox's
  "all" as a boolean and the subscription's kinds as an array of strings;
- the **console's query proxy** (`kernel/webui/webui_proxy.go`) coerces the
  numeric route arguments before forwarding them, because a query string only
  carries text.

One gap that binding found: the proxy's numeric list knew `limit`, `since_ms`,
`older_than_days` and `count`, but not the **day windows** the reaper and the
skill hygiene read (`idle_days`, `stale_days`). A query-carried day count
arrived as text, and the handler — which reads a number — ignored it, so the
console's own filter was silently the default. The proxy coerces those two
keys now, and the door names a day count that is not a number.

### 2.4 The gates

Two guards hold the line:

- the **registry's own rule** — every property of every request body the daemon
  accepts declares a type
  (`kernel/controlplane/request_schema_ratchet_test.go`,
  `TestRequestSchemasNameEveryPropertyBeyondTheConsole`). W5.8's console-scoped
  guard stays as the narrower claim it was.
- the **door's own test** — a mistyped body on the CLI/SDK surface never
  reaches the handler and never reaches the audit
  (`kernel/controlplane/request_typed_admission_cli_test.go`): 24 bodies over
  **13** packages (workboard, pulse, board, okr, seats, taste, edict, journal,
  providers, tools, update, webhook, steer's own route).

## 3. The mutations

Three reverted mutations prove the guards fire:

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the workboard's `max_attempts` loses its type | the registry ratchet — *"workboard_policy: the request property "max_attempts" declares no type: {}"* |
| M2 | the pulse approval flag becomes a number | the door's test — the body passes the door and the handler answers *"pulse is disabled"* instead of the type error |
| M3 | the OKR key result's target becomes a string | the door's test — the door stops refusing the text the CLI sends |

Every mutation was reverted; the tree carries none of them.

## 4. Preserved behaviour

- The **services** did not change: every reader still reads what it read. A
  lenient reader still reads leniently when the service is called directly; the
  registry's door is what now refuses a mistyped body.
- The **wire protocol** did not change: only the declared type of a field
  moved. A caller that sends the right type is unaffected.
- The **console** did not change: its request rows and the OpenAPI document
  carry the same fields as before (`openapi -check` and `sdkgen -check` are
  green) — the surface this slice typed is not the console's JSON body
  surface.
- What **did** change is what a mistyped body does: it is refused at admission
  with the field's name and both types, instead of reaching the handler as a
  silent default. That is the whole point of the slice, and the tests that
  asserted the old leniency were rewritten to assert the new truth (the
  handler's own error message became the door's, and the cases that were
  proving a *fallback* now prove a *refusal*).

## 5. Verification

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
- frontend: `tsc --noEmit` — 0 errors; `knip` — clean; `kernel/webui/dist` is
  in sync (the bundle was rebuilt and did not change).
- frontend `vitest` — **no regression**, and the machine's failures are
  pre-existing and flaky. This slice changes **no frontend source**
  (`git diff HEAD -- frontend` is empty and there is no untracked file under
  `frontend/`), so the browser's suite was measured four times in full: twice
  in the slice worktree and twice in a **clean `HEAD` worktree**
  (`git worktree add --detach`, the same `node_modules`), compared by failing
  test id:

  | run | failing tests |
  |---|---|
  | slice worktree, run 1 | **80** |
  | slice worktree, run 2 | **71** |
  | clean `HEAD` worktree, run 1 | **76** |
  | clean `HEAD` worktree, run 2 | **76** |

  The count moves and so does the **identity**: the same clean tree produced
  two different failing sets (5 tests each way), and the slice tree's two runs
  differ by 16. The **stable core is 70** failures present in all four runs —
  in both trees — and every one of them fails on `localStorage`
  (`Cannot read properties of undefined (reading 'clear')`): Node **26.13**
  on this machine ships `localStorage` behind `--localstorage-file`
  (*"localStorage is not available because --localstorage-file was not
  provided"*), so the theme/storage-backed component tests cannot run here.
  Every test that differed between the two trees **passes when its file is
  run alone** in both trees. The failures are a property of this machine and
  of parallel load, not of the slice; they are present at `HEAD` and the
  slice neither adds nor removes one.

Measured: the registry's request surface **122 → 0** operations with an untyped
request property and **418 → 0** untyped request properties, across **29**
daemon packages; **324 of 324** operations carry a fully typed request body.
