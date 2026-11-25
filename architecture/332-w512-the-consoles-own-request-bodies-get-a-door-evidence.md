# W5.12 — the console's own request bodies get a door

## 1. What was wrong

The console's **own** endpoints — the ones that are not operation bindings:
the login surface, the rollback surface, the file manager, the voice surface —
answer the **browser**, and their request bodies were read into whatever the
handler happened to use.

The console's own endpoint list (`kernel/contract/opapi/native.go`) names **21**
endpoints with their path, method, tier and mutation flag. Six of them take a
request body, and **none** of those six said what the body looks like. The
bodies were read into three different shapes:

| endpoint | the body it reads | the shape it reads it as |
|---|---|---|
| `login` | `{"password"}` | an anonymous struct |
| `rollback_apply` | `{"id"}` | an anonymous struct |
| `tts` | `{"text"}` | an anonymous struct |
| `files_mkdir` | `{"path","parents"}` | **a `map[string]any`** |
| `files_rename` | `{"from","to"}` | **a `map[string]any`** |
| `files_delete` | `{"path","recursive"}` | **a `map[string]any`** |

So a mistyped body was decided by Go's decoder — and in a **map** body, not
decided at all: the handler read `body["path"].(string)` and a number became
the **empty string**, silently, and the mutation was sent to the daemon
anyway.

Measured on the tree at `origin/main` (`85eb284b`), with the console's own
password gate and voice backend configured:

| the body | the old answer |
|---|---|
| `POST /api/login {"password":5}` | 400 `bad request` |
| `POST /api/login {"password":null}` | 401 `invalid password` |
| `POST /api/rollback/apply {"id":5}` | 400 `invalid JSON body: json: cannot unmarshal number into Go struct field .id of type string` |
| `POST /api/files/mkdir {"path":5}` | **200** — the path became `""` and the mutation was sent anyway |
| `POST /api/files/mkdir {"parents":"yes"}` | **200** — the flag became `false` |
| `POST /api/files/rename {"from":5,"to":"y"}` | 400 `from and to are required` |
| `POST /api/files/delete {"recursive":"yes"}` | **200** — the flag became `false` |

Three of the seven — the ones that went through a map — were **accepted** with
a silent default. The other four were refused, but in four different voices,
none of them the operation registry's: the registry says *"operation
files_mkdir input: $.path has wrong type: got number, want string"*.

## 2. What was built

### 2.1 The contract names the console's own bodies

`opapi.NativeRoute` gained a `Body` field: the JSON Schema of the request body
that endpoint takes, empty when it takes none. It sits in the SAME table as
the path, the method and the tier — one promise per endpoint:

```go
{Name: NativeFilesMkdir, Path: "/api/files/mkdir", Method: "POST", Mutation: true,
	Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"path":{"type":"string"},"parents":{"type":"boolean"}}}`)},
```

Two rules keep the schema the registry's shape, and they are the same two the
REST surface's rows follow:

- **no `required` list** — the door checks TYPES; presence is the handler's
  business (an absent path is still the handler's *"from and to are
  required"*);
- **`additionalProperties: true`** — a schema is the stable **subset** of a
  body, so the daemon may read more than the contract names.

### 2.2 The console has a door, and the door's check is one place

`kernel/webui/request_door.go` gives the console the same door the REST
surface got in W5.11, and the CHECK itself now lives in the transport
(`kernel/httpserver.DecodeJSONBody`): one place where a request body becomes
a value, so both surfaces read the same bytes the same way.

The refusal carries the registry's words, and the sentence has **one home**
now — `schema.InputError` in `kernel/platform/schema`:

```go
// InputError wraps a failed request body in the operation registry's words:
// "operation <op> input: <err>".
```

The registry itself wraps its own input failures with it
(`kernel/app/dispatch.go`), the REST door uses it, and the console uses it —
so a caller learns one language whichever surface the mistake was made on.

The console answers the refusal in its **JSON envelope**, so the browser can
show the sentence: the browser's `errMsg` reads `{"error": …}` and falls back
to a bare `HTTP 400` for a plain-text answer — the endpoints that answer text
lose their message in the browser today, and a refusal the door writes does
not have to.

### 2.3 The file manager's bodies are typed now

The three file endpoints read their bodies into named structs instead of a
`map[string]any`:

```go
type fileMkdirBody struct {
	Path    string `json:"path"`
	Parents bool   `json:"parents"`
}
```

The `body["path"].(string)` pattern — where a wrong type became a zero value
and the caller was never told — is gone from the console.

## 3. The measurement

| | before | after |
|---|---|---|
| console-native endpoints | 21 | 21 |
| console-native endpoints that take a JSON body | 6 | 6 |
| bodies the contract names a shape for | **0** | **6** |
| endpoints behind a door | **0** | **6** |
| request fields checked before the handler reads them | **0** | **9** |
| the seven measured mistyped bodies accepted with a silent default | **3** | **0** |
| the refusal's words | four different voices | the registry's one sentence |
| places that write the registry's refusal sentence | 2 | **1** |

The nine fields are `login` (1), `rollback_apply` (1), `files_mkdir` (2),
`files_rename` (2), `files_delete` (2) and `tts` (1).

### 3.1 What did not change

- The **status codes** of the console's own answers. A body over the route's
  cap is still a **400** (the console's convention; the REST surface answers
  413), and the message is clearer than the decoder's.
- The **malformed-body answers** are still each endpoint's own ("bad request"
  for the login, *"expected JSON body {"text": …}"* for the voice surface, the
  decoder's words for the file manager).
- The **early answers** are still early: the login endpoint answers *"no
  password gate configured"* without reading a body, and the voice surface
  answers *"text-to-speech is not configured"* without reading one. The door
  sits where the decoder sat, so an endpoint that never read a body still
  never reads one.
- The **valid bodies** behave exactly as before (measured: the login still
  mints a session, the rollback still answers "checkpoint not found", the file
  manager still forwards the mutation).
- The **generated client** did not move: the console's path unions and the
  OpenAPI document are byte-identical (`tools/openapi -check`).

## 4. The guards

| # | the guard | what it holds |
|---|---|---|
| 1 | `kernel/contract/opapi`: `TestTheConsoleNativeBodiesAreTypedSchemas` | every body the list names is a schema the door can check: every property declares a type, no `required` list, an undeclared field stays allowed |
| 2 | `kernel/contract/opapi`: `TestTheConsoleNativeBodyLookupAnswersTheList` | the lookup the door uses answers the schema the list states, and an endpoint that takes no body answers none |
| 3 | `kernel/webui`: `TestTheConsoleNativeBodiesAgreeWithTheContract` | the console's structs and the contract's schemas name the **same fields**, in both directions, **with the same type** — and every schema belongs to an endpoint the console reads a body for |
| 4 | `kernel/webui`: `TestTheConsoleDoorRefusesAMistypedBody`, `TestTheConsoleDoorRefusesANullField` | a mistyped field — and a null — is refused with the registry's words, for every field of every console-native body |
| 5 | `kernel/webui`: `TestTheConsoleDoorLetsAContractBodyThrough` | the door does not close the console |

Guard 3 is the one that holds the contract and the console together; guard 4
states its expectation from the **console's own struct**, not from the
contract's schema, so a schema that mistyped a field cannot agree with itself
and pass.

## 5. The mutations

Five mutations, five caught, all reverted (`.temp_files/w512_mutations.py`):

| # | the mutation | the guard that caught it |
|---|---|---|
| M1 | the door is not wired into the file manager's mkdir | *"the console-native endpoint "files_mkdir": a mistyped "path" said "json: cannot unmarshal number into Go struct field fileMkdirBody.path of type string", want "operation files_mkdir input: $.path has wrong type: got number, want string""* |
| M2 | the contract renders a boolean as a `string` | *"the console-native endpoint "files_mkdir": a mistyped "parents" said …, want "operation files_mkdir input: $.parents has wrong type: got string, want boolean""* |
| M3 | the console's refusal drops the registry's wrapper | *"the console-native endpoint "login": a mistyped "password" said "$.password has wrong type: got number, want string", want "operation login input: $.password has wrong type: got number, want string""* |
| M4 | the contract's schema stops allowing an undeclared field | *"the console-native endpoint "files_mkdir" does not allow an undeclared field — a schema is the stable subset of a body"* |
| M5 | the shared core stops checking the schema | *"the console-native endpoint "files_rename": a mistyped "from" said "json: cannot unmarshal number into Go struct field fileRenameBody.from of type string", want …"* |

The first run of M2 and M3 **escaped**, and the escape was the point: the
door's test derived its expectation from the contract's schema, so a schema
that mistyped a field agreed with itself — and the refusal's wrapper was
inside the error the test compared, so dropping it changed nothing. Both were
fixed (the expectation now comes from the console's own struct, and the
schema error is wrapped once, where the message is written), and both
mutations fire now.

## 6. Preserved behaviour

- **The wire protocol did not change.** A body that fits the contract reaches
  the handler exactly as before.
- **The SDKs did not change.** Not a line of Python, TypeScript or Rust moved
  — the REST contract's rows did not move either.
- **The browser did not change.** The generated client is byte-identical; the
  browser's own bodies are typed already.
- **The registry did not change.** Its door was already there; this slice gave
  the registry's refusal sentence a single home and gave the console the same
  door.

## 7. Verification

- `gofmt -l kernel cmd plugins tools sdk` — clean.
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — green.
- `go run ./tools/archcheck` — clean (340 packages, 0 violations).
- `go run ./tools/deadcodecheck` — clean (33 public SDK findings and 4
  cross-package test seams allowlisted).
- `go run ./tools/depscheck` — clean.
- `go run ./tools/sdkgen -check` — in sync.
- `go run ./tools/structure-md -check -out .project/STRUCTURE.generated` — in
  sync.
- `go run ./tools/openapi -check` — in sync.
- `go run ./tools/changelog-lint` — clean.
- `go run ./tools/docclaimscheck -base origin/main` — clean.
- `go run ./tools/docsindex` — clean.
- Python SDK: `python -m unittest discover -s tests` — **51** tests, green.
- TypeScript SDK: `npm test` (tsc + node:test) — **31** tests, green.
- Rust SDK: `cargo fmt --check` + `cargo test` — clean, **39** tests, green.

Measured: console-native bodies the contract names a shape for **0 → 6**;
endpoints behind a door **0 → 6**; request fields checked before the handler
reads them **0 → 9**; the measured mistyped bodies accepted with a silent
default **3 → 0**; places that write the registry's refusal sentence **2 → 1**.
