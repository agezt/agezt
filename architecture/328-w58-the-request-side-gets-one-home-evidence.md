# W5.8 — the request side gets one home

## 1. What was wrong

W5.5 gave the console its own endpoints one home, and W5.6–W5.7 gave the
browser the daemon's **response** rows. What was left was the other half of
the same dictionary: the daemon's **request** side.

The console's route catalogue has **70** routes that take a JSON body — the
contract the browser sends against. Measured over that surface
(`.temp_files/w58_surface.py`):

| surface | before | after |
|---|---|---|
| request bodies with an untyped property | **56** | **0** |
| untyped properties in the console's request surface | **337** | **0** |
| OpenAPI request properties that declare a type | — | **382** of **382** |
| generated request rows | **48** | **48** |
| request rows that are entirely / partly `any` | — | **0** / **0** |
| the browser's request rows | **57** rows, **205** fields | **57** rows, **204** fields |
| the browser's request fields typed `any` | **162** | **0** |

(The field count is one lower because the renderer now **shares** an identical
row: the provider's OAuth status request is the channel's OAuth status request,
so it is published as an alias of it instead of a second copy.)

An empty property schema — `{}` — says "anything". So the daemon's contract
for **337** fields said nothing at all, the browser's generated request row
carried `any` for **162** of its fields, and a mistyped field reached the
handler as a **silent default**: `from: true` on the board became an empty
sender, `enabled: "TRUE"` on a standing order became *false*, `assure: "wrong"`
became *zero*. The operator saw a successful edit and the daemon had done
something else.

## 2. What was built

### 2.1 The rule: the field's read type

Every property of every body the browser can send now names the type the
handler reads it as. The rule has three parts:

- the type is the field's **read** type — string, number, boolean, object,
  array;
- a **union** only where the handler genuinely accepts two meaningful shapes
  (`enabled`: a boolean, or the text "true"/"1" — the flag a form sends; a
  count: a number, or the digits a query string carries);
- **no null and no junk arms**. A mistyped field is a usage error, not a
  default.

**19** daemon packages carry the change: board, channels, chat, configcenter,
council, data, market, persona, redaction, research, roster, routing, runs,
schedule, settings, skill, standing, tools, workflow.

### 2.2 The door moved, and the door's message is better

A mistyped body is refused **at admission** — before the handler runs and
before the audit sees it. The refusal names the field and both types:

```
operation board_send input: $.from has wrong type: got boolean, want string
```

That is the same message shape the repo already uses for a mistyped *cursor*
(`$.cursor has wrong type: got null, want string`), so the request side now
answers the way the rest of the daemon answers.

The repo's doctrine (**admission before effects**) is untouched: the
dispatcher was not changed. The door was already there; what changed is that
the request contract finally says what the door should check.

### 2.3 The renderer was dropping half a union

`tools/openapi/types.go` rendered a schema that declares **several** types by
taking the first arm it understood: `["string","array"]` came out as
`string[]`. Every union in the request surface was therefore published with
one arm missing — the browser's row said something the daemon never promised.

The renderer now renders **every arm** (`renderArm`), and the `null` arm is no
longer detected by looking for the word "null" in the rendered text — which
had been swallowing the nullable arms of nullable maps and objects. The
side effect is visible and correct: the daemon's chain map is
`Record<string, string[] | null> | null`, and the browser was **null-blind**
about it.

### 2.4 The browser became null-aware

The chain map's null arm was real: the daemon's registry may send a null map
and a null chain list. The browser read it as if it could not happen — **17**
compile errors, in **6** files, are the compiler's list of the places where it
did:

- `src/lib/chains.ts` — the chain map, its helpers and the label;
- `src/features/workflows/components/Chains.tsx` — the chain editor;
- `src/components/ModelPicker.tsx`, `…/agentdetail/ModelTab.tsx` — the pickers;
- `src/lib/configbackup.ts` — the config bundle's prompts, which were
  `unknown[]` while the daemon's `prompts_set` takes `Record<string, any>[]`.

The browser's view of the chain map is now the **narrowed** map it works with
(`ChainRegistry`), and the null arm is normalised at the fetch boundary
(`?? {}`) — the pattern the ACP inventory already used.

### 2.5 One daemon bug the binding found

`plan_json` was published as an **object**. The daemon reads it as a JSON
**string** — its own message says so: *"args.plan_json must be a JSON string"*.
With the object published, the door would have refused **every** plan
submission. The type is `string` now, for the plan and for the plan refine.

## 3. The guards

**`kernel/controlplane/request_schema_ratchet_test.go`** — every property of
every body route the console binds must declare a type, and the type must be
one of the JSON types. The guard walks the **console's route catalogue**, not
the registry's whole surface: a body nobody can send from the browser is not
the browser's contract.

**`frontend/src/app/api/requestrows.ratchet.test.ts`** — the browser's side,
with two teeth: no request row may carry a bare `any` (a free-form object —
`Record<string, any>` — is a typed shape and is allowed), and the browser may
not spell a request row of its own.

**`kernel/controlplane/request_typed_admission_test.go`** — the door itself:
a mistyped body is refused before the handler runs and before the audit sees
it, for the board, the council and the research harness.

The payoff is the **compiler**: the browser's `postJSON` takes
`RequestOf<P>` — the request row the daemon declares for that path — so
**46** of the **48** body paths are now checked at build time against the
daemon's own contract. Only two paths stay free-form (the provider's OAuth
logout and the provider reload, whose bodies the daemon does not describe).

## 4. The mutation evidence

Three mutations, three caught, all reverted:

| # | Mutation | Caught by |
|---|---|---|
| M1 | The research request's `verify` goes back to an empty schema (`{}`) | the Go ratchet — *"research_ask (/api/research/ask): the request property "verify" declares no type: {}"* |
| M2 | The generated request row's `text` becomes a bare `any` | the frontend ratchet — *"/api/board/send: BoardSendRequest.text is any"* |
| M3 | A hand-written copy of the daemon's `BoardSendRequest` appears in the browser | the frontend ratchet — *"app/api/mutationcopy.ts: BoardSendRequest"* |

## 5. Verification

- `go test ./...`, `go vet ./...`, `gofmt -l` — clean.
- `npx tsc --noEmit` — **0** errors (was **17** right after the binding).
- `npx vitest run` (whole suite, same machine, same conditions, compared by
  failing test id against a clean `HEAD` worktree): baseline **79** failing,
  slice **73** failing, **6** fixed, **0 new**.
- `npx knip` — clean.
- `archcheck` (340 packages, 0 violations, 0 allowlisted import violations,
  0 allowlisted call sites), `deadcodecheck`, `depscheck`, `sdkgen -check`,
  `structure-md -check`, `changelog-lint`, `docclaimscheck`, `docsindex`,
  `openapi -check` — green.
- The frontend bundle was rebuilt and committed with the slice
  (`kernel/webui/dist`).
- `staticcheck` is CI-only (no prebuilt binary on this machine); it did not
  run here.

## 6. What did not change

- **The daemon's wire protocol**: the shape of every body is the same; only
  the **declared** type of a field changed. A caller that already sent the
  right type sees no difference at all.
- **`kernel/app/dispatch.go`**: the admission order is the repo's doctrine and
  was not touched.
- **The `required` lists**: presence ("absent" vs "null") stays in the
  handler. A field the daemon always sends says so in the schema; a field it
  may omit is still the handler's to notice.
- The **registry's CLI-only surface**: **122** operations outside the console's
  route catalogue still carry an untyped request property. They are the
  SDK/CLI's surface, not the browser's, and they are the next slice's
  material.

## 7. What comes next

The console's body contract is typed and guarded on both sides. What is left
is the **registry's own surface**: the **122** operations the browser cannot
reach, whose request properties are still `any` — the CLI and the SDK send
them, and the SDK's own request rows have the same hole the browser's had.
The same treatment (the registry's schema for the request side, a client that
sends the row the daemon declares, and a guard that holds the line) is the
next end of "the dictionary gets one home".
