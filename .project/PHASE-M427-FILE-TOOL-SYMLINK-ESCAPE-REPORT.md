# M427 — file tool search/glob symlink escape (HIGH security)

## Context
Review of the agent-reachable tools (file, http, browser) — the model controls their
inputs, so input validation IS the security boundary. The `http` and `browser` tools
and `netguard` were found clean (host-allowlist strips port/userinfo, AllowAll still
goes through netguard, redirects re-checked per hop, response bodies capped). One HIGH
escape in the file tool, plus a MEDIUM OOM.

## The bug (HIGH — arbitrary file read)
`plugins/tools/file/file.go`: `read`/`write`/`append`/`replace`/`stat`/`delete` all
route through `resolve()`, which `EvalSymlinks`-resolves the target and rejects
anything outside `t.root`. But `doSearch` and `doGlob` walk the tree with
`filepath.WalkDir` and read/enumerate every non-dir entry **without** re-validating
symlinks. `WalkDir` lstat-types entries (it never follows a link), so a symlink-to-file
has `d.IsDir() == false` and passes the dir-skip; `os.ReadFile(p)` then *follows* the
link and reads the out-of-root target, returning the matched lines to the model.

A prompt-injected agent with the file tool can point an in-root symlink (or use a
pre-existing one — `node_modules`, a mounted config) at `/etc/passwd` or
`…/.aws/credentials` and `search` with a broad pattern to exfiltrate its contents,
fully bypassing the otherwise-correct `resolve()` confinement. The reviewer reproduced
it with a passing PoC.

## The fix
New `(*Tool).entryEscapesRoot(p, d)`: for a symlink entry (`d.Type()&fs.ModeSymlink`),
`EvalSymlinks` the path and report whether the real target leaves `t.root` (an
unresolvable link is also treated as escaping). `doSearch` and `doGlob` skip any entry
for which it returns true — `search` no longer reads out-of-root targets, `glob` no
longer enumerates them. (`WalkDir` already does not descend symlinked directories, so
only the direct symlink-entry case needed guarding.)

Also (MEDIUM, OOM): `doSearch` and `doReplace` read whole files into memory with no cap
(`doRead` caps at 256 KB but these did not). Both now bound the per-file read at a new
`MaxScanBytes` (8 MiB) — `search` skips an oversized file, `replace` errors — so an
agent can't grow a workspace file to gigabytes and OOM the daemon by grepping/replacing.

## Verification
- **`plugins/tools/file/file_test.go`** `TestContainment_SearchGlobDoNotFollowSymlinkOutsideRoot`:
  an in-root symlink to an out-of-root secret is NOT read by `search` (no hit) and NOT
  enumerated by `glob`. (Skips where the platform disallows symlink creation.)
  - **Negative control:** removing the `entryEscapesRoot` guard from `doSearch` → the
    secret content leaks as a hit → FAIL. Restored byte-identical.
- The existing containment suite (read/write/replace symlink + traversal rejection)
  still passes.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2285** passing (was 2284; +1). CHANGELOG
  Security entry.

## Review status
`http`/`browser` tools + `netguard` reviewed clean. The remaining LOW from the review
(a `doWrite` create-new path lacks `O_NOFOLLOW`, a narrow TOCTOU only reachable via a
*concurrent* Invoke on the same tool) is deferred — it needs platform-tagged syscall
code and the window requires concurrency the single-Invoke tool model doesn't expose.
The MCP bridge findings (separate review) are handled in M428.
