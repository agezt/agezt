# W5.4j — the platform gains a streaming atomic writer, and the backup uses it

## What the slice is

The last two sites in the CLI's raw-write ratchet, and the platform addition
one of them needs.

`internal/atomicfile` could write a file whose bytes were **already in
memory**. The backup archive is a **tar+gzip stream** — produced as it is
written, and potentially large — so it could not use that API at all. It used
`os.Create`, which truncates the target in place: a crash mid-stream left a
**torn archive**, and `agt restore` reading it would write **half a home**
into the destination before failing on the truncated gzip stream.

## The API

```go
func WriteStream(path string, mode os.FileMode, fn func(w io.Writer) error) error
```

The same guarantees as `WriteFile` — a **unique** temp file in the target's
directory, fsynced, chmodded, renamed over the target — with two decisions
worth naming:

- **The callback shape makes the commit atomic by construction.** There is no
  "forgot to close" state, and the error path cannot leak a temp file. `fn`
  receives an `io.Writer`, not the `*os.File`, so it cannot close or sync a
  handle the platform owns.
- **The Windows last resort is different, and deliberately so.**
  `WriteFile`'s last resort re-writes the bytes because they are **in
  memory**. A stream's bytes are not — but they are **on disk**, one rename
  away in the temp file. So the fallback **copies the temp onto the target**
  instead of re-writing from memory. The same guarantee, streamed.

`WriteStream` lives in `internal/atomicfile` (L0 foundation), not in
`kernel/platform/filestore`: it is the *same mechanism* with the *same*
guarantees, and the platform's stated principle is one way to do each
infrastructural thing. A second atomic writer in another package would
recreate the divergence this package exists to kill.

## What the backup gained — and what it lost

The old code:

```go
f, err := os.Create(outPath)
...
man, werr := createBackup(home, f, headSeq, headHash, time.Now())
closeErr := f.Close()
if werr != nil {
    os.Remove(outPath)      // ← the cleanup
    ...
}
```

The new code:

```go
err = atomicfile.WriteStream(outPath, 0o600, func(w io.Writer) error {
    var werr error
    man, werr = createBackup(home, w, headSeq, headHash, time.Now())
    return werr
})
```

**The `os.Remove(outPath)` cleanup is gone — and that is the point, not a
detail.** Under a plain `os.Create` the target had already been truncated, so
removing it was the only way to avoid leaving a half-written archive. Under an
atomic write the target still holds the operator's **previous good archive**,
and removing it would destroy exactly what they need. The cleanup was not
merely redundant; it became actively harmful, and the atomic writer is what
made it so.

**The mode changed**: `os.Create` applied the umask (≈`0644`); the archive is
now `0600`. The archive is secret-free by construction — the include subtrees
are `journal` and `catalog`, and the secrets live outside them — but a backup
is the operator's whole activity history, and `0600` is the posture every
other file in the product writes with. The mode bits do not exist on Windows,
where the archive's protection is the directory's.

## The proof

### Permanent tests — 7 new

| Test | What it holds |
|---|---|
| `TestWriteStream_WritesAndReplaces` (`internal/atomicfile`) | the published bytes are the ones `fn` produced, an existing target is replaced wholesale, and no temp is left behind |
| `TestWriteStream_FailureLeavesTheTargetUntouched` | **the guarantee the whole slice is for**: a stream that fails does not touch the target — including a target that already existed — and leaves no scratch file |
| `TestWriteStream_Mode` | the mode is applied to the temp before the rename — **POSIX only**, and the test says so instead of pretending on Windows |
| `TestWriteStream_MissingDir` | the parent directory must exist — the same rule `WriteFile` has, and the two must agree |
| `TestWriteStream_ConcurrentSameTarget` | concurrent streams to one target never interleave: the final content is one writer's bytes |
| `TestAFailedBackupLeavesThePreviousArchiveIntact` (`cmd/agt`) | **the operator's last good archive survives a failed backup** — and no scratch file is left next to it |
| `TestTheBackupArchiveIsWholeAndPrivate` | the published archive reads back whole (the inspector can parse it) and carries the `0600` posture — **POSIX only** |

The failure injection in the backup test is a **symlink to a path that does
not exist**: the archive walk Lstats it (the link itself exists), sees it is
not a directory, and then fails to READ it, mid-stream. A permission trick
would not work — the mode bits do not exist on Windows — and where symlinks
are unavailable the test says so instead of pretending.

### The mutation run — 5 mutations, 5 caught

Two independent passes with **byte-identical** logs.

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the backup archive goes back to a plain `os.Create` — a crash mid-stream leaves a torn archive | the file-grained test **and** the gate |
| M02 | the backup removes the output when it fails again — the previous good archive is destroyed | `TestAFailedBackupLeavesThePreviousArchiveIntact` |
| M03 | the streaming writer publishes the partial stream when the stream fails | `TestWriteStream_FailureLeavesTheTargetUntouched` |
| M04 | the streaming writer leaves its temp file behind when the stream fails | `TestWriteStream_FailureLeavesTheTargetUntouched` |
| M05 | the streaming writer creates the parent directory — `WriteFile` does not, and the two must agree | `TestWriteStream_MissingDir` |

The calibration reported **one SKIP**, and it was the mutation's fault: the
anchor for M02 matched **two** places in `backup.go` (the `resolveHome`
error block and the write's error block are textually identical), so the
harness refused to guess. The anchor now includes the line before it.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 1 allowlisted call site remains** |
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
raw-write cmd/agt 2   →   raw-write cmd/agt 1
```

## What remains

**One** site: the doctor's writability probe
(`cmd/agt/doctor_ops_env.go:34`) — a transient file nobody reads, where
`os.CreateTemp` is the right shape.

But the honest note matters more than the number: **the rule counts only
`os.WriteFile` and `os.Create`, so `os.CreateTemp` is invisible to the
gate.** The count would drop without a new guarantee being enforced, and a
zero would then mean "no raw writes **the counter can see**" rather than "no
raw writes". The fix is a follow-up slice that **extends the rule** to
`os.CreateTemp` and `os.OpenFile(O_CREATE)` — and that extension will
surface an honest count elsewhere in the tree, which is the point.

And the two import edges are untouched: `controlplane → runtime` (37 verbs, a
redesign) and `selfrepair → runtime` (needs a runs port that does not exist
yet).
