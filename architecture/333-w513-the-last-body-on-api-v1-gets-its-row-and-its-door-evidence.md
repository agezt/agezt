# W5.13 — the last body on `/api/v1` gets its row and its door

## 1. What was wrong

`POST /api/v1/update/apply` was the one body on `/api/v1` the contract did not
describe. The endpoint reads `{"version","sha256","url","notes"}` into
`updateApplyArgs` — a struct of its own — while the REST contract names **no
row** for it. So the daemon's request door (W5.11) had **no schema to measure
that body against**: it was the one body on the surface the door could not
reach, and what happened to a mistyped body was decided by Go's
`encoding/json` — a sentence about a Go struct field, not the operation
registry's.

Measured on the tree at `origin/main` (`fb8ba22f`), with the self-update
service wired:

| the body | the old answer |
|---|---|
| `{"version":42,…}` | 400 `invalid_json` — *"json: cannot unmarshal number into Go struct field updateApplyArgs.version of type string"* |
| `{"version":null,…}` | 400 `missing_fields` — the null became `""` and the answer was about a **missing** field |
| `{"notes":null}` | **200** `{"applied":true}` — the null was accepted and the update went through |
| `[]` | 400 `invalid_json` — *"json: cannot unmarshal array into Go value of type restapi.updateApplyArgs"* |
| an empty body | 400 `empty_body` |
| a body over 64 KB | 413 `body_too_large` |
| a body that fits | 200 `{"applied":true}` |

So a mistyped body was refused in a **third voice** — not the registry's, and
not the REST surface's — and a **null** was not refused at all: `{"notes":null}`
was an update that was applied.

The second half of the hole was named in the handoff note. A request row in the
REST contract is a row the **generated SDK catalogs** carry, and the catalog
rendered **every row in the vocabulary** — so naming the self-update body
would have put a fourth request body into three SDKs' public API, and the
SDKs' own ratchets would have demanded that every SDK **build** it, although
the SDKs deliberately do not expose the self-update surface (`Endpoint.SDK`
says so about the endpoint).

## 2. What was built

### 2.1 The contract names the self-update body

The REST contract gained a row for it, next to the other request rows:

```go
{
	Name: RowUpdateApply,
	Doc:  "The body of a self-update request.",
	Fields: []Field{
		{Name: "version", Type: TypeString, Required: true, Doc: "The release version to install."},
		{Name: "sha256", Type: TypeString, Required: true, Doc: "The SHA256 of the release archive."},
		{Name: "url", Type: TypeString, Required: true, Doc: "Where the release archive is fetched from."},
		{Name: "notes", Type: TypeString, Doc: "The release notes."},
	},
	SDK: false,
}
```

The route and the endpoint name it (`Request: RowUpdateApply`), so the door
renders it as a JSON Schema exactly like every other request row — no
`required` list, `additionalProperties: true`, the registry's type words.

### 2.2 The SDK catalogs carry the **SDK contract's** rows

The contract now says, per row, whether the SDK catalog carries it — the same
decision the endpoint table already made per endpoint:

```go
// SDK marks a row the SDK catalog carries — a shape the SDK contract
// publishes.
SDK bool
```

Three functions carry the scopes, and they live in the contract — one home:

| the function | the set it holds |
|---|---|
| `Rows()` | the whole vocabulary (17 → **18** rows) |
| `RequestRows()` | every request row of the **surface** (3 → **4**) — the door's set |
| `SDKRows()` | the rows the **SDK catalog** carries (17 → 17) |
| `SDKRequestRows()` | the request rows of the **SDK contract** (3 → 3) |

`tools/sdkgen` renders `SDKRows()` and treats `SDKRequestRows()` as the
request set, and the SDKs' own ratchets walk `SDKRequestRows()`. The
self-update body is in the contract and out of the SDKs — so **not a line** of
Python, TypeScript or Rust moved (`tools/sdkgen -check` is clean).

### 2.3 The door measures it

The handler now reads its body through the surface's door, so the body is
measured against the contract's row before the handler reads it:

```go
var args updateApplyArgs
if !decodeRequestBody(w, r, restapiapi.EndpointUpdateApply, &args) {
	return
}
```

The refusal carries the registry's words — `operation update_apply input:
$.version has wrong type: got number, want string` — and a **null** is refused
instead of becoming the zero value.

## 3. The measurement

| | before | after |
|---|---|---|
| rows in the contract's vocabulary | 17 | **18** |
| rows the SDK catalog carries | 17 | 17 |
| contract rows the SDK catalog does **not** carry | 0 | **1** |
| request rows on the surface | 3 | **4** |
| endpoints behind a door | 3 | **4** |
| request fields checked before the handler reads them | 11 | **15** |
| bodies refused with the registry's sentence | 11 | **15** |
| nulls refused (not silently zero) | 11 | **15** |
| request rows the SDK contract speaks | 3 | 3 |
| lines of Python, TypeScript or Rust that moved | 0 | **0** |

The four new fields are `update_apply`'s: `version`, `sha256`, `url`,
`notes`.

### 3.1 What did not change

- **The endpoint's own answers.** An empty body is still `empty_body`, a
  missing field is still `missing_fields`, a disabled self-update still
  answers `update_disabled` — and it still answers it **before** it reads a
  body, so a daemon with the update disabled never reads one.
- **The valid bodies.** A body that fits the contract still reaches the
  handler and still applies the release.
- **The SDKs.** The three catalogs are byte-identical; the golden request
  fixture changed by **one line** — its note now says whose request rows it
  holds.
- **The wire protocol.** The daemon still reads the same struct; only the
  refusal before it changed.

### 3.2 The two refusals that did change

| the body | before | after |
|---|---|---|
| a body over the route's cap | 413 `body_too_large` — *"request body must be under 64 KB"* | 413 `request_too_large` — *"request body exceeds the size limit"* |
| a malformed body | 400 `invalid_json` — the decoder's words | 400 `invalid_request` — *"invalid JSON body: …"* |

Both moved **toward the surface's own voice**: the oversize answer is the one
the run and mailbox endpoints already give (W5.11 changed the ack endpoint's
oversize answer the same way), and the malformed body is now the surface's
`invalid_request` instead of this endpoint's private `invalid_json`. The
status codes did not move: both were 413 and 400 before, and both are 413 and
400 now.

One corner moved with them: an empty body sent with a **chunked** encoding
(so the daemon cannot see its length up front) is answered by the surface's
`invalid JSON body: EOF` instead of this endpoint's `invalid_json`. The
ordinary empty body — the one with a known length — still answers
`empty_body`.

## 4. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/restapiapi`: `TestEverySDKEndpointExchangesPublishedRows` | a row an SDK endpoint exchanges is published to the SDKs, or the catalog would miss a shape the SDKs speak |
| 2 | `kernel/contract/restapiapi`: `TestTheSelfUpdateBodyStaysOutOfTheSDKCatalog` | the self-update body stays out of the SDK catalogs and inside the surface's request rows — and the door still has a schema for it |
| 3 | `kernel/contract/restapiapi`: `TestTheRouteAndTheEndpointAgreeOnTheRequestRow` | the route and the endpoint name the same body, or the door would measure a body the route does not take |
| 4 | `kernel/restapi`: `TestTheRequestRowsAgreeWithTheDaemonsRequestBodies` | the contract's row and the daemon's struct name the same fields **with the same types** — a name-only ratchet let a mistyped field through |
| 5 | `kernel/restapi`: `TestTheRequestDoorRefusesAMistypedField`, `TestTheRequestDoorRefusesANullField` | the door refuses a mistyped field and a null with the registry's words, for every field of every request row — the self-update body included |
| 6 | `tools/sdkgen`: `TestTheSDKCatalogCarriesNoRequestRowOfANonSDKEndpoint` | the SDK catalog carries no request body of an endpoint the SDKs do not expose |
| 7 | `kernel/restapi`: `TestTheGoldenRequestFixtureHoldsEverySDKRequestRow` | the golden request fixture holds exactly the SDK contract's request rows — and nothing else |

Guard 4 is the one that was **added** here: the ratchet compared the field
**names** of the contract's row and the daemon's struct, so a contract that
mistyped a field (`sha256` as a boolean) agreed with itself and passed.

## 5. The mutations

Five mutations, five caught, all reverted (`.temp_files/w513_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the door measures the wrong endpoint's row | *"the "update_apply" endpoint: a mistyped "version" said "invalid JSON body: json: cannot unmarshal number into Go struct field updateApplyArgs.version of type string", want "operation update_apply input: $.version has wrong type: got number, want string""* |
| M2 | the contract mistypes the self-update row (`sha256` as a boolean) | *"the contract's "update_apply" row names "sha256" as "bool", the "update_apply" endpoint reads it as string"* |
| M3 | the refusal drops the registry's wrapper | *"the "update_apply" endpoint: a mistyped "version" said "$.version has wrong type: got number, want string", want "operation update_apply input: $.version has wrong type: got number, want string""* |
| M4 | the contract's schema stops allowing an undeclared field | *"the "update_apply" endpoint refused a body carrying an undeclared field"* |
| M5 | the SDK catalog carries the self-update body | *"the SDK catalog carries the "update_apply" row — it is the request body of the "update_apply" endpoint the SDKs do not expose, so no SDK would build it"* |

M2 is the mutation the **name-only** ratchet could not see: the door's own
test derives its expectation from the contract's row, so a row that mistyped a
field agreed with itself. The type half of the ratchet is what closed it.

## 6. Preserved behaviour

- **The wire protocol did not change.** A body that fits the contract reaches
  the handler exactly as before, and the handler still reads its own struct.
- **The SDKs did not change.** Not a line of Python, TypeScript or Rust moved;
  the generated catalogs are byte-identical.
- **The browser did not change.** The console's own bodies were typed in
  W5.12 and its generated client is byte-identical (`tools/openapi -check`).
- **The registry did not change.** Its door was already there; this slice gave
  the last body on `/api/v1` the same door.

## 7. Verification

- `gofmt -l kernel cmd plugins tools sdk` — clean.
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — green.
- `go run ./tools/archcheck` — clean.
- `go run ./tools/deadcodecheck` — clean.
- `go run ./tools/depscheck` — clean.
- `go run ./tools/sdkgen -check` — in sync (the SDK catalogs did not move).
- `go run ./tools/structure-md -check -out .project/STRUCTURE.generated` — in
  sync.
- `go run ./tools/openapi -check` — in sync.
- `go run ./tools/changelog-lint` — clean.
- `go run ./tools/docclaimscheck -base origin/main` — clean.
- `go run ./tools/docsindex` — clean.
- Python SDK: `python -m unittest discover -s tests` — green.
- TypeScript SDK: `npm test` (tsc + node:test) — green.
- Rust SDK: `cargo fmt --check` + `cargo test` — clean.

Measured: request rows on the surface **3 → 4**; endpoints behind a door
**3 → 4**; request fields checked before the handler reads them **11 → 15**;
bodies refused with the registry's sentence **11 → 15**; nulls refused
**11 → 15**; contract rows the SDK catalog does not carry **0 → 1**; lines of
SDK code that moved **0**.
