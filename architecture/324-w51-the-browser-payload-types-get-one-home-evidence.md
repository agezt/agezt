# W5.1 — the browser's payload types get one home

> **Slice:** W5.1 (second half) · **Date:** 2026-11-22 · **Base:** `f252a23c` (main)
> **Kind:** a surface is derived instead of written; seven real bugs die on the way.
> **Follows:** W5.1's first half ([318](318-w51-openapi-and-the-generated-client-evidence.md)),
> which gave the API document and the browser's client one home. That slice's
> §10 said what this one does: *"the browser's payload types are still
> hand-written: 160 frontend interfaces describe what the registry already
> describes."*

---

## 1. What was wrong

The registry has described every payload since the operations were bound. The
browser had **352** hand-written `interface` declarations describing the same
payloads, and **217** call sites named their payload type by hand:

```ts
const r = await getJSON<AgentListResponse>("/api/agents", {});
```

Nothing connected the two. The browser's types were written by reading the
daemon's behaviour, and where the reading was wrong the browser was wrong in a
way no gate could see. **Nine** of those mistakes were real bugs — panels that
rendered empty, a version that read "—", a decision that was never shown — and
the compiler found every one of them the moment the types stopped being
hand-written.

| Surface | How the payload was described | Count |
|---|---|---|
| the daemon's registry | JSON Schema, derived from the Go types (`schema.FromType`) | 324 operations |
| the browser | hand-written `interface` declarations | **352** |
| the browser's call sites | an explicit type argument | **217** in 67 files |

## 2. What was built

### 2.1 `tools/openapi/types.go` — the TypeScript renderer (L7)

A JSON Schema → TypeScript renderer, next to the OpenAPI producer that already
read the same schemas. It writes a third artifact:

- **`frontend/src/app/api/types.generated.ts`** — 358 interfaces + 143 aliases,
  4 457 lines, **203 response paths** and **65 request paths**.

The naming is derived, never hand-picked: an operation's response type is its
operation name in PascalCase plus `Response`; a nested object is its parent plus
the property name. Two operations that carry the same shape share one
declaration — the second is an `export type` alias — so the browser names the
shape it means without the file repeating it.

### 2.2 The client is bound to the **path**, not to a type name

The client used to be generic over the payload:

```ts
getJSON<T = any>(path: ReadPath, options?): Promise<T>
```

Every call site had to name `T`, and a wrong name was a wrong name — the
compiler could not check it against the console's own answer. The client now
answers with the shape the path carries:

```ts
getJSON<P extends ReadPath>(path: P, options?): Promise<ResponseOf<P>>
postJSON<P extends JsonPath | NativePath>(path: P, body: RequestOf<P>): Promise<ResponseOf<P>>
postAction<P extends WritePath | NativePath>(path: P, params?): Promise<ResponseOf<P>>
```

`ResponseOf<P>` and `RequestOf<P>` are conditional types over the generated
`OperationResponseMap` / `OperationRequestMap`. The browser's call site now
says **which path it asks**, and the path says what comes back.

**All 217 type arguments are gone** (66 files). The only generic parameters
left in the tree are the client's own six declarations.

### 2.3 The browser's 132 interfaces became **views**

A view keeps the browser's own shape — which fields it reads, and whether it
reads them as required — while every field's **type** comes from the registry:

```ts
export type AgentProfile = {
  id?: AgentListResponseProfilesItem["id"];
  slug: AgentListResponseProfilesItem["slug"];
  name?: AgentListResponseProfilesItem["name"];
  noise_policy?: AgentListResponseProfilesItem["noise_policy"];
  …
};
```

A nested browser type stays a nested browser type: the view of a view, narrowed
the same way. The daemon decides what a field is; the browser decides which
fields it looks at.

**352 → 219** hand-written interfaces. The 133 that went away are 132 views and
one type deleted outright (`SkillFile`, whose fields the daemon does not send —
see §4).

The views were **matched, not guessed**. A candidate had to be the root of a
path the interface's users fetch, or live in the tree of one, or have exactly
the same shape; and every field's type had to be compatible with the wire's.
Every interface that had no such candidate was **left alone** — they are the
browser's own (a nav row, a toast, a modal's props), not a payload.

## 3. The view's four measured decisions

Four things in a view are choices, not facts. Each was measured on the real
browser tree, by the number of call sites it breaks:

| Question | Measured | Chosen | Why |
|---|---|---|---|
| does the wire's `null` survive? | keep it: **149** broken call sites; drop it: **38** | **drop** | the daemon sends `null` for a nil slice, and the browser has always read a null field and a missing field the same way (`\|\| []`, `?.`, `??`). The exact nullable contract is not lost: `docs/openapi.json` is generated from the same schemas and keeps every null arm. |
| does the registry's `required` list survive? | — | **yes** | a field the daemon always sends is required in the browser too; the registry's list is the contract, not a suggestion. |
| is a request body strict? | — | **yes** | the daemon rejects a body that lacks a required field, so the browser learns that from the compiler instead of from a 400. |
| does a shape get one declaration? | — | **yes** | 143 aliases; the browser names the shape, not the operation that happened to declare it first. |

The null rule is the one that cost the most to learn and it is worth stating
plainly: **the first design kept the null arms and the browser broke in 149
places**; the browser's own code was written against a contract that said
`undefined`, and folding `null` into absent is what that code already did.

## 4. The nine real bugs the compiler found

The drift was not theoretical. These are the surfaces that were reading a field
the daemon does not send — each one is a panel that was silently empty:

| Surface | The browser read | The daemon sends | What the operator saw |
|---|---|---|---|
| `MissionControl.tsx` | `/api/status` → `version`, `revision` | `/api/version` → `agezt`, `revision` | the build row read "—" forever |
| `MissionControl.tsx` | `/api/agents` → `agents` | `profiles` | "Agents: 0" on a fleet of twelve |
| `Jarvis.tsx` | `ready`, `status` (a string) | `enabled`, `status.operational_label` | every agent read "offline" |
| `Approvals.tsx` | `/api/approvals` → `history` | `/api/approvals_log` → `approvals` | the granted/denied history was always empty |
| `skills/page.tsx` | `s.uses`, `m.wins` | `s.metrics.uses`, `m.successes` | every skill read "never used" |
| `FilesTab.tsx` | `f.path`, `f.size` (objects) | `files: string[]` | every bundled file was a blank line |
| `Research.tsx` | `answer`, `sub_questions[].question`, `verified_claims[].verified` | `markdown`, `sub_questions: string[]`, `claims[].verdict` | the whole Research panel was empty |
| `ChatGPTSignInCard.tsx`, `channels/page.tsx` | `r.error` | `authorize_url`, `state`, `ok` | a failed sign-in had no message |
| `workflows/page.tsx` | `nodes` on a list row | `node_count`, `edge_count` | the workflow canvas opened blank |

Two more were **the browser narrowing the daemon on purpose**:

- `AgentProfile.kind` was `"system" | "custom" | "subagent"`; the wire says
  `string`. The daemon decides, so the browser's narrowing is a promise it never
  made.
- `AgentProfile.noise_policy.min_notify_severity` was
  `"info" | "warning" | "critical"`; the wire says `string`.

Both are now the daemon's type, and the browser's own closed sets
(`ApplyMode` = live/restart, the severity buckets it renders) stay where they
belong — at the point where the browser maps the daemon's string onto its own
vocabulary.

## 5. The guards

| Guard | Where | What it holds |
|---|---|---|
| `TestTheGeneratedArtifactsAreInSync` | `tools/openapi` | the committed document, client **and types** are exactly what the sources produce |
| `go run ./tools/openapi -check` | the tree | the same, as a byte gate |
| the **type checker** | `frontend` | a field the daemon renames breaks the browser, at the view and at the call site |
| **knip** | `frontend` | the generated file is an entry, so its 501 exports are not "unused"; the browser's own exports stay the browser's |

## 6. The mutation evidence

Two mutations were applied and reverted. Each one renames a field **in the
daemon** — not in the generated file — so the whole chain has to carry the
rename for the browser to survive it.

**Mutation A — the answer's name moves.** `kernel/app/research/research.go`:
`json:"markdown"` → `json:"answer_md"`. The generator was re-run and the
browser's type checker was asked:

```
src/features/knowledge/components/Research.tsx(25,34): error TS2339:
  Property 'markdown' does not exist on type 'ResearchAskResponse'.
```

**Mutation B — the profile's instructions move.**
`kernel/contract/agentapi/agentapi.go`: `json:"instructions,omitempty"` →
`json:"directives,omitempty"`. The browser reads that field through an indexed
view, so the break lands on **both** sides of the view:

```
src/lib/agentshared.ts(18,48): error TS2339:
  Property 'instructions' does not exist on type 'AgentListResponseProfilesItem'.
src/features/agents/components/agentdetail/MindTab.tsx(100,56): error TS7006:
  Parameter 'ins' implicitly has an 'any' type.
```

The first error is the **view** breaking — the browser's own type says a field
the daemon no longer sends. The second is the **call site** breaking, because
the field's type is no longer known.

Both mutations were reverted, the artifacts were regenerated, and
`go run ./tools/openapi -check` and `npm run typecheck` were re-run green.

## 7. Verification

| Gate | Result |
|---|---|
| `gofmt -l kernel cmd tools internal` | clean |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go test ./...` | green — no package failed |
| `archcheck` | green — 340 packages, 0 violations, 0 allowlisted imports, 0 allowlisted calls |
| `deadcodecheck` | green — 33 public SDK findings and 1 cross-package test seam allowlisted, as before |
| `depscheck` | green — 24 core dependencies, all justified |
| `sdkgen -check` | green |
| `openapi -check` | green — the document, the client and the types are in sync |
| `structure-md -check` | green — the codemap is current |
| `changelog-lint` | green |
| `docclaimscheck` | green |
| `docsindex` | green |
| `staticcheck` | **not runnable on this machine** — the tool is a prebuilt, SHA-256-verified binary in CI and is not installed here. The slice's Go changes are one new file and four edits in `tools/openapi`, all of which `go vet` accepts. |
| `cargo fmt --check` / `cargo test` (Rust SDK) | green — 4 tests + 3 doc-tests |
| `npm run typecheck` | green |
| `npx knip` | green |
| `npx vitest run` | the suite's red is the machine's, unchanged in kind — see §8 |

## 8. The frontend suite's baseline

The frontend's vitest suite was already red before this slice (this machine's
Node build has no `localStorage`, and the suite's `beforeEach` hooks call
`localStorage.clear()`). The comparison is by **test id**, against a baseline
measured on a clean `HEAD` worktree with the same `node_modules`:

| Run | Suites | Tests | Failed |
|---|---|---|---|
| baseline (`f252a23c`, clean worktree) | 540 | 1 443 | 77 |
| this slice (run 1) | 540 | 1 443 | 85 |
| this slice (run 2) | 540 | 1 443 | 84 |

The two runs of the same tree failed **different** sets (11 tests in run 1, 9 in
run 2, with only 2 in common), and the files that failed pass **146/146** when
they are run on their own. That is this machine's load-induced flakiness, the
same class the previous W5.1 slice measured and recorded — not a regression.

Four test files **were** changed, because they asserted the drifted contract:
`Jarvis.test.tsx` (the roster's `profiles`), `Approvals.test.tsx` (the history
comes from `/api/approvals_log`), `MissionControl.test.tsx` (the build comes
from `/api/version`) and `Research.test.tsx` (the answer is `markdown`, the
claims carry a verdict). Each now mocks what the daemon actually sends.

## 9. What did not change

- The daemon's operations, the registry, and the 324 operation contracts. The
  browser's view of a payload is a **projection** of the contract, never a
  second contract.
- The console's HTTP behaviour, the request shapes, the verbs and the paths.
- The browser's own types — the interfaces that describe no payload (a nav
  row, a toast, a modal's props, a canvas node) are untouched.
- `docs/openapi.json`, which remains the document for a caller outside the
  browser, with every null arm and every required field.

## 10. What comes next

The browser's payload types are derived now. Two things are left in the same
family:

1. The browser's **request bodies** are typed by the generated request map
   (65 paths), but the browser still builds some of them as
   `Record<string, unknown>` — the memory import and the world import. They
   are typed now, but the *helpers* that build them (a JSON file importer, a
   shape checker) are still the browser's own.
2. The browser's **view types** are still projections the browser names
   (`AgentProfile`, `ReaperReport`, `ApiOrder`). The daemon's own row types are
   the truth; a browser that wants a narrower view says so with `Pick`. A
   future slice could name the daemon's rows directly and let `Pick` be the
   browser's only narrowing tool.
