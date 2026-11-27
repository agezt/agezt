# W5.31 — the console's file-tree answer has one home: the row is declared, the route names it, and the browser binds to it

## 1. What was wrong

The file manager is a **console-native** endpoint: the console answers
`/api/files/tree` itself, not a daemon operation. Every other console-native
answer had been given a row — the auth meta, the login, the logout, the SSE
token, the voice status, the rollback list, the workflow hook — but the
**file tree was the one left over**, and it had no row at all. So the row was
spelled by hand on **both** sides of the wire.

In the console:

```go
// kernel/webui/files_route.go
type fileNode struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"` // "dir" | "file"
	Size       int64  `json:"size,omitempty"`
	ModifiedMS int64  `json:"modified_ms,omitempty"`
}

type fileTreeResponse struct {
	Root  string     `json:"root"`
	Nodes []fileNode `json:"nodes"`
}
```

And in the browser:

```ts
// frontend/src/lib/files.ts
export interface FileNode {
  name: string;
  path: string; // POSIX, no leading slash, root = ""
  type: "dir" | "file";
  size?: number;
  modified_ms?: number;
  // Children are loaded on demand by useFileTree; absent on a leaf.
  children?: FileNode[];
}

export interface FileTreeResponse {
  root: string;
  nodes: FileNode[];
}
```

Two spellings of the same row, in two languages, with nothing to say they were
the same row — and **two drifts** already in them:

- the browser promises a **`children`** member the daemon never sends (the
  daemon lists ONE directory at a time; the browser walks the tree by loading
  a folder's children when it opens it — that structure is the browser's own,
  but the copy did not say so);
- the browser **narrows** `type` to `"dir" | "file"`, a promise the daemon's
  row never made (the daemon says `"dir"` or `"file"` today; the row does not
  narrow it, so a third kind of entry would not break the reader).

And the native route named **no row at all** — so the catalogue had nothing
to render, `ResponseOf<"/api/files/tree">` resolved to `any`, and the browser
had nothing to bind to.

## 2. What was built

### 2.1 The row is declared in the console's contract

`kernel/contract/consoleapi` gains the file manager's answer, beside the
console's other rows:

```go
type FileTree struct {
	Root  string     `json:"root"`
	Nodes []FileNode `json:"nodes"`
}

type FileNode struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size,omitempty"`
	ModifiedMS int64  `json:"modified_ms,omitempty"`
}
```

The members are declared in the order the wire speaks them, so the served
bytes did not move.

### 2.2 The console serves the contract's row

The console's own structs became **aliases**, the way the rollback store's
checkpoint became one in W5.26:

```go
type fileNode = consoleapi.FileNode
type fileTreeResponse = consoleapi.FileTree
```

An alias cannot drift: a copy cannot grow next to it, because there is no
copy.

### 2.3 The route names the row

```go
{Name: NativeFilesTree, Path: "/api/files/tree", Method: "GET",
	Answer: consoleapi.FileTree{}},
```

**Nine** native routes name their row now, and the console's ratchet drives
every one of them: a member the console sends that the row does not carry
fails, and a member the row always carries that the console never sends fails
too.

### 2.4 The catalogue carries it, and the browser binds to it

The catalogue renders the row and binds the path to it:

```ts
export interface FilesTreeResponseNodesItem {
  modified_ms?: number;
  name: string;
  path: string;
  size?: number;
  type: string;
}

export interface FilesTreeResponse {
  nodes: FilesTreeResponseNodesItem[];
  root: string;
}
```

The browser's view is the catalogue's row plus the one member that is the
browser's own:

```ts
export type FileTreeResponse = Omit<FilesTreeResponse, "nodes"> & {
  nodes: FileNode[];
};

export type FileNode = FilesTreeResponseNodesItem & {
  // Children are loaded on demand by useFileTree; absent on a leaf.
  children?: FileNode[];
};
```

The browser names **no field** of the daemon's row by hand, and it says which
member is its own.

### 2.5 The guards

**A — the compiler.** The browser's view is **pinned** to the catalogue's row,
so a hand-written copy cannot come back without the typecheck saying so:

```ts
type _fileNodeIsTheRow = Expect<
  Equal<FileNode, FilesTreeResponseNodesItem & { children?: FileNode[] }>
>;
```

**B — the console's ratchet** (`kernel/webui/native_rows_test.go`). The route
names a row, so the console must answer with that row — in both directions.

**C — the console's byte pin** (`kernel/webui/console_rows_test.go`). The
answer's bytes are pinned: the empty listing, and a listing with one entry
(the node's members, in the wire's order, up to the timestamp that moves on
every write).

**D — the browser's catalogue test** (`app/api/console_rows.test.ts`). The
console-native path resolves to its row — the runtime half and the
compile-time half, which says so if the catalogue ever stops binding the path
to the row.

## 3. What was measured

| | before | after |
|---|---|---|
| browser declarations that spell the file-tree row | **2** | **0** |
| console's own spellings of the row (structs) | **2** | **0** |
| console's aliases of the contract's row | **0** | **2** |
| native routes that name the row they answer with | **8** | **9** |
| file-tree rows in the browser's catalogue | **0** | **2** |
| the served bytes | — | **unchanged** |

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w531_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the browser's file-tree view spells the row by hand again.**

```
src/lib/files.test.ts(50,3): error TS2344: Type 'false' does not satisfy the constraint 'true'.
```

**M2 — the console answers a member the row does not carry.**

```
native_rows_test.go:151: the native route "files_tree" answers the member "symlink", which the row it names does not carry
```

**M3 — the contract's row drops a member the console used to send.**

```
console_rows_test.go:107: the file tree answer's bytes moved:
```

**M4 — the native route stops naming the row it answers with.**

```
src/app/api/console_rows.test.ts > binds the paths the browser's console-native views read
AssertionError: expected undefined to be 'FilesTreeResponse'
```

**M5 — the catalogue stops rendering the file-tree row.**

```
src/app/api/console_rows.test.ts > binds the paths the browser's console-native views read
AssertionError: expected undefined to be 'FilesTreeResponse'
```

## 5. What is untouched

- **No daemon behaviour changed.** The console serves the same bytes — the
  byte pin says so; the browser reads them.
- **The published OpenAPI document did not move** (`docs/openapi.json`): it
  describes the operation-bound surface, and the file manager is not an
  operation binding.
- **No layer rule moved.** `archcheck` still reports 0 violations; the
  contract's row is in the console's own contract, and the console imports
  it.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, and the frontend's
  `npm test`, `npm run typecheck`, `npm run build`, `npm run deadcode`.

## 6. What remains open

- **The console's error envelopes are still maps** — a shape, not a row.
- **The console's own answers are not in `docs/openapi.json`**, and the
  document says so. A caller outside the browser still has no published shape
  for `/api/authmeta` or `/api/files/tree`.
- **The `/hooks/` path is a prefix**, not a path the browser calls: the hook's
  row is rendered, but the browser never fetches it.
- **The browser's file manager still builds a stub tree** when the route
  answers 404 — the stub is the browser's own vocabulary, and it stays.
