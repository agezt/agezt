# W5.4k — the rule learns to see `os.CreateTemp`, and the forbidden-call ratchet is zero

## What the slice is

The last site in the CLI's raw-write ratchet, and — the part that matters —
the **rule itself**.

W5.4j left the ratchet at one site and named the honest reason in the record:

> the rule counts only `os.WriteFile` and `os.Create`, so `os.CreateTemp` is
> invisible to the gate

That is not a formatting note. It is the difference between a ratchet that
measures and a ratchet that only counts. A package could move its write to a
temp file, the number would fall, and **no new guarantee would exist anywhere**
— the count was reporting less than it claimed to report.

## The rule, before and after

```go
// before
CallRawWrite: {"os": {"WriteFile", "Create"}},
// after
CallRawWrite: {"os": {"WriteFile", "Create", "CreateTemp"}},
```

`os.CreateTemp` belongs in the set because a package that creates its **own**
temp file is hand-rolling the atomic writer: temp → fsync → chmod → rename is
exactly the mechanism the rule exists to have in exactly one place. The one
implementation is `internal/atomicfile` — **layer 0**, exempt as the foundation
it is.

So the extension does not create debt; it **reveals** it. The measurement
before the change:

| `os.CreateTemp` site | Layer | Verdict |
|---|---|---|
| `internal/atomicfile/atomicfile.go:26` | L0 | the writer itself — exempt |
| `internal/atomicfile/atomicfile.go:96` | L0 | the writer itself — exempt |
| `plugins/tools/file/file_write.go:173` | L6 | **a second implementation of the atomic writer, in a package** |

One real violation, and it was a **duplicate writer**: the file tool kept its
own copy of the whole mechanism (temp → write → fsync → close → chmod →
rename) instead of calling the canonical one.

## The two writes that moved

### 1. The doctor's writability probe (`cmd/agt/doctor_ops_env.go`)

The probe writes a file into the daemon's home directory and removes it again.
It was the last raw write in the CLI, and it is now a write through the
platform's atomic writer like every other file the CLI writes.

The probe's whole **purpose** is to be a faithful sample of what a write to
that directory does — so it goes through the same writer a real file would.
It also stopped being a special case: `os.WriteFile` applied its mode only at
creation and truncated in place; the canonical writer creates a **unique**
temp, fsyncs, chmods and renames, so the probe is exactly the thing it claims
to test.

### 2. The file tool's writer (`plugins/tools/file/file_write.go`)

The plugin kept its own copy of the atomic writer, with one reason on record:

> Kept local (not internal/atomicfile) so the `writeAll` seam above can inject
> mid-write failures in tests.

The seam is real and worth keeping — it is how the plugin proves that a
**failed** write leaves the original file intact. But the seam did not need a
second implementation to survive: `atomicfile.WriteStream` takes a
**callback** (`func(w io.Writer) error`), which is exactly the shape a
mid-write injection needs. The plugin's write is now a stream:

```go
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return atomicfile.WriteStream(path, perm, func(w io.Writer) error {
		_, err := writeAll(w, data)
		return err
	})
}
```

The `writeAll` seam survives (it now takes an `io.Writer`), the guarantee is
the canonical one, and the plugin gained the Windows fallback the canonical
writer already had.

## The proof

### Permanent tests — 5 new, 2 rewritten

| Test | What it holds |
|---|---|
| `TestTheRawWriteRuleSeesATempFile` (`tools/archcheck`) | **the blind spot, pinned**: a package that hand-rolls a temp writer is counted as a raw write — the rule sees what it used to be blind to |
| `TestCountFileCalls` (`tools/archcheck`, updated) | the rule's count moves from 2 to 3 with the `os.CreateTemp` line in its fixture |
| `TestTheCLIHasNoRawWrites` (`cmd/agt`, rewritten) | **the CLI has no raw write left** — file-grained, so a package that stays under the count cannot hide a new raw write in a file the count no longer names |
| `TestTheDoctorProbeLeavesNothingBehind` (`cmd/agt`) | a successful probe leaves the daemon's home **exactly as it found it** — no probe file, no scratch file |
| `TestTheDoctorProbeFailsWhenTheTargetIsBlocked` (`cmd/agt`) | the probe **fails** when the file it writes cannot land — and leaves no scratch file behind |
| `TestTheFileToolHasNoRawWrites` (`plugins/tools/file`) | the plugin has no raw write left — file-grained, same reason as the CLI's |
| `TestTheReplaceOpLeavesNoTempFile` (`plugins/tools/file`) | a completed `replace` leaves exactly the file it wrote — no scratch file beside it |

The probe's failure test blocks the target with a **non-empty directory**, and
that choice is the cross-platform one: on POSIX the rename fails with
`EISDIR`, and on Windows the writer's fallback cannot remove it either (a
non-empty directory cannot be removed), so the write fails on both. A
permission trick would not do — the mode bits do not exist on Windows, and a
read-only directory there does not stop the write.

### The mutation run — 6 mutations, 6 caught

Two independent passes with **byte-identical** logs (SHA-256
`79F8E61D…32A256`).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the doctor probe goes back to a plain `os.WriteFile` | the file-grained test **and** the gate |
| M02 | the rule stops counting `os.CreateTemp` — the blind spot comes back | `TestTheRawWriteRuleSeesATempFile` |
| M03 | the probe stops removing its file — the doctor litters the daemon home | `TestTheDoctorProbeLeavesNothingBehind` |
| M04 | the file tool swallows the write error — a failed write publishes a truncated file | `TestAtomicWriteFile_PreservesOriginalOnWriteFailure` |
| M05 | the file tool stops streaming — the write is no longer the seam the atomicity proof injects into | `TestAtomicWriteFile_PreservesOriginalOnWriteFailure` |
| M06 | the file tool writes raw — a second implementation of the atomic writer | the file-grained test **and** the gate |

M01 and M06 are the pair that shows the two instruments working together: the
**gate** counts a package, the **test** counts files, and both name the
regression when a raw write comes back.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## The ratchet

```
raw-write cmd/agt 1   →   (the forbidden-call allowlist is EMPTY)
```

`tools/archcheck/calls-allowlist.txt` now holds nothing but its header. **The
forbidden-call ratchet is zero** — one of the two numbers W5.4 asks for.

The **import** ratchet stands at **2**:

- `adapter-bypass kernel/controlplane -> kernel/runtime` — 37 verbs, a redesign
- `cross-module kernel/modules/agents/selfrepair -> kernel/runtime` — needs a
  runs port that does not exist yet

## What remains — and it is a different kind of remainder

The rule still cannot see **`os.OpenFile` with a write flag**. The selector
table is not able to look at the *flags*: `os.OpenFile(path, flag, perm)` takes
the mode as an argument, so a selector-based rule would count every
`os.OpenFile` — including the read-only ones.

The measurement (non-test sites, write flags):

| Site | What it is |
|---|---|
| `kernel/agentgw/secret.go:70` | `O_CREATE\|O_EXCL\|O_WRONLY` — a secret file |
| `kernel/configcenter/audit.go:150` | `O_APPEND\|O_CREATE\|O_WRONLY` — the audit log |
| `kernel/tenant/tenant.go:85` | `O_CREATE\|O_EXCL\|O_WRONLY` — a tenant file |
| `kernel/update/update_verify.go:253,304` | download + verification temp |
| `kernel/journal/recover.go:106`, `kernel/journal/journal_io.go:140,253` | journal segments (`O_EXCL`) |
| `kernel/platform/filestore/lock.go:37` | the lock file |
| `plugins/tools/codeexec/artifacts_helpers.go:89` | a code-execution artifact |
| `cmd/agt/backup_restore.go:179` | the restored file (`O_EXCL`) |
| `kernel/controlplane/update_control.go:20` | the update marker |

**Twelve** sites, plus four more whose flag is a **variable** (the three
`openFileNoFollow` wrappers in the file plugin and `journal_seg.go`) and
therefore cannot be classified by looking at the call at all.

Counting them needs a **flag-aware** rule: the counter has to read the second
argument, recognise `O_WRONLY` / `O_RDWR` / `O_CREATE` / `O_APPEND` /
`O_TRUNC` in an `|`-expression, and decide what to do with a flag it cannot
resolve (a variable). That is a separate gate slice, and it is deliberately
**not** smuggled into this one: a rule that guesses is worse than a rule that
does not exist, because a guess that silently under-counts is exactly the
blind spot this slice just closed.

Two more notes worth carrying:

- The file plugin's `doWrite` (the plain `write` op) still opens the target
  with `O_CREATE|O_TRUNC` through `openFileNoFollow` — that path is
  deliberately **not** atomic (it streams, and it honours append mode), and it
  is one of the `os.OpenFile` sites above.
- The rule's **exemption unit is the package**, and it stays that way. The
  file plugin could not be exempted as "a generator that writes its own
  output": it writes the **agent's** files, and the exemption would have
  hidden every other raw write in the package with it.
