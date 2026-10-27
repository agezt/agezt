# W5.4g — the CLI's file writes are visible, and the signing key is written atomically

## What the slice is

The last blind spot in the forbidden-call gate.

`tools/archcheck` enforces three rules on the call sites themselves: `exec`
(a child process must go through the sandbox launcher), `http-client` (a
dial-out must go through the guarded client) and `raw-write` (a file must be
written through `internal/atomicfile` or the store framework). The rule
set is fixed in `tools/archcheck/calls.go`; `layers.json`'s "calls" array
says where each rule's calls are legitimate.

The scan skipped two layers outright:

```go
if !ok || cl.Layer == LayerFoundation || cl.Layer == LayerRoot {
    continue
}
```

with the comment *"Packages in layers 0 and 7 are always exempt: foundation
helpers ARE the implementations, and binaries/tooling compose."*

**Layer 0 is right.** The foundation packages ARE the implementations — the
atomic writer itself writes.

**Layer 7 was not.** A binary legitimately spawns a process and calls an
API — that is its job. But a **file** a CLI writes is a file the daemon may
read back, and a plain `os.WriteFile` skips the atomic writer (fsync +
unique temp + rename) and the store framework's permissions.

## What the blind spot hid

The CLI is `cmd/agt`, and it had **seventeen** `os.WriteFile` / `os.Create`
call sites. Two of them mattered more than the other fifteen:

- **`cmd/agt/market_publish.go:103`** wrote the operator's **Ed25519 signing
  key** — the marketplace publishing identity — with a plain
  `os.WriteFile(prefix+".key", []byte(priv+"\n"), 0o600)`. A crash mid-write
  leaves a **truncated key file**, and a truncated signing key is an
  unrecoverable loss: the operator's whole publishing identity is gone, with
  no error to show for it. And the `0600` mode is meaningless on Windows,
  where the POSIX mode bits do not exist.
- **`cmd/agt/doctor_mesh.go:34,47,62,71`** wrote the daemon's own
  **`memory.json`** — a real store the daemon reads back — with the same
  plain writer, bypassing the store framework that owns it.

The other fifteen are artifact and report writes (`agt artifact get`,
`agt journal export`, `agt peers fetch`, `agt plugin new`, `agt plugin
registry`, `agt skill export`, `agt backup`, and the doctor's writability
probe). They are not stores, but a half-written export is a broken export,
and `cmd/agt/rollback_catalog.go:90` still used the **fixed-name temp**
pattern (`path + ".tmp"` then rename) that `internal/atomicfile` replaced in
M478 — two concurrent writers race on that name.

## The rule change

The exemption became **rule-aware** instead of blanket:

```go
func layerExempt(rule string, layer int) bool {
	switch layer {
	case LayerFoundation:
		return true
	case LayerRoot:
		return rule == CallExec || rule == CallHTTPClient
	}
	return false
}
```

A binary keeps the two exemptions that are about its job. It loses the one
that is not.

The rule change alone was not enough: it made **every** layer-7 package
visible, and most of them should not be. `tools/archcheck` writes its own
allowlists, `tools/sdkparity` writes `docs/SDK-PARITY.md`,
`tools/structure-md` writes the generated structure files,
`tools/changelog-split` splits the changelog, `tools/jsonschemagen`
generates a schema — **a generator IS the implementation of its own output**,
which is exactly why layer 0 is exempt. And `kernel/internal/testfixtures` is
a test helper. So `layers.json`'s "calls" array gained them explicitly:

```json
{"rule": "raw-write", "allow": ["kernel/platform/filestore/...", "tools/...", "kernel/internal/testfixtures"]}
```

**The CLI is not in that list.** After the change the gate reports exactly
one package: `raw-write cmd/agt`.

## The ratchet, honestly

The forbidden-call ratchet was **zero** after W5.4a. It is now **one
entry**:

```
raw-write cmd/agt 15
```

That is a step back in the headline number, and it is the honest reading:
**the zero was a zero because the rule was not looking.** The import-edge
ratchet has the same shape — it started at seven and is at two — and a
ratchet that starts at fifteen is a ratchet that may only go down. The
remaining fifteen are counted, named and visible; before this slice they
were none of those.

The slice removed the two sites that are not "a file the operator asked
for": the signing key and its public half.

## The fix

`cmd/agt/market_publish.go` writes both halves through the platform's one
canonical writer:

```go
if err := atomicfile.WriteFile(prefix+".key", []byte(priv+"\n"), 0o600); err != nil {
```

The writer writes to a **unique** temp file in the target's directory,
fsyncs it, chmods it and renames it into place — so the file on disk is
either the whole key or not there, and two concurrent keygens never race on
a shared temp name.

The `0600` posture is the store framework's (`filestore.FilePerm`). On
Windows the mode bits do not apply, and the repo's own answer to a secret
that must not be readable at all is the **encrypted vault**
(`kernel/creds`, AES-256-GCM + PBKDF2) — not a mode bit. That is a product
decision, not a refactor, and it is left to the owner.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `TestTheRootLayerIsNotExemptFromRawWrite` (`tools/archcheck/calls_test.go`) | the rule: the root layer is exempt from `exec` and `http-client`, **not** from `raw-write`; the foundation is exempt from everything; the other layers from nothing |
| `TestTheMarketKeygenWritesAKeyThePublishPathAccepts` | the key the keygen writes is a key `market.PrivateKeyFromHex` accepts — the keygen and the publish path are only ever run together |
| `TestTheMarketKeygenLeavesNoTempFileBehind` | the atomic writer leaves no temp behind |
| `TestTheMarketKeygenKeepsTheKeyPrivate` | the key file is `0600` and the public key `0644` — **POSIX only**; Windows has no mode bits, and the test says so instead of pretending |
| `TestTheMarketKeygenRejectsAnUnknownArgument` | an unknown argument is an error and writes nothing |

`agt market keygen` had **no test at all** before this slice.

### The mutation run

**6 mutations, 6 CAUGHT**, in two independent passes with **byte-identical**
logs.

The product of this slice is the **gate's reach**, so the proof is the gate
itself: each mutation must make `tools/archcheck` fail **and name the site
that regressed**.

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the root layer is exempt from `raw-write` again — the CLI goes blind | the gate reports `raw-write cmd/agt` as no longer occurring |
| M02 | the foundation layer is exempt from every rule again | the gate reports `internal/atomicfile`'s own writes |
| M03 | the signing key goes back to a plain `os.WriteFile` | the gate reports `raw-write cmd/agt` above its allowance |
| M04 | the public key goes back to a plain `os.WriteFile` | the gate reports `raw-write cmd/agt` above its allowance |
| M05 | the generators lose their exemption | the gate reports `raw-write tools/...` |
| M06 | the test fixtures lose their exemption | the gate reports `raw-write kernel/internal/testfixtures` |

The calibration pass reported **one FAILED-OTHER**, and it was the
mutation's fault: its anchor (`"tools/...", `) also matched the **layer
pattern** `{"pattern": "tools/...", "layer": 7}`, so the mutation produced a
malformed `layers.json` and the gate failed with a JSON parse error instead
of the expected finding. The anchor was narrowed to the whole allow list.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 15 allowlisted call sites remain** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK — the documentation index agrees with the docs/ tree |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What remains

The ratchet holds fifteen sites, and they are not the same kind of thing:

| Sites | What they are |
|---|---|
| 4 — `doctor_mesh.go` | the daemon's own **`memory.json`**: a store the daemon reads back. The sharpest of the fifteen — the store framework owns that write. |
| 1 — `rollback_catalog.go` | the **fixed-name temp** pattern the atomic writer replaced |
| 1 — `backup.go` | `os.Create` plus a **streaming** write; the atomic writer has no streaming API yet, so this one needs a platform addition first |
| 9 — the rest | artifact and report exports (`artifact get`, `journal export`, `peers fetch`, `plugin new`, `plugin registry`, `skill export`, the doctor's writability probe) |

The doctor's store repair is the one to take next: it is a daemon store, and
the store framework already owns how it is written.
